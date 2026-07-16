// Package rollups maintains derived summary tables from the append-only
// events table. The events table stays the source of truth (never mutated
// here); everything this package writes is rebuildable derived state:
//
//	DELETE FROM user_rollups;
//	UPDATE rollup_watermarks SET last_event_id = 0 WHERE name = 'user_rollups';
//
// user_rollups.events_miles is the pure pairing base — it excludes the live
// subscriber bonus and manual corrections, so it reads lower than users.miles
// (that delta is a drift alarm, not a bug). user_rollups.extra_miles captures
// SUM(events.extra_miles_earned) over logout + correction events: the sub-grant,
// 5%-bonus, and manual-correction portion the pairing can't see, so reconstructed
// display miles ≈ events_miles + extra_miles.
// users.miles remains the authoritative display number; these columns are for
// audit, reconciliation, and cross-platform aggregation.
package rollups

import (
	"context"
	"log/slog"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	"github.com/adanalife/tripbot/pkg/scoreboards"
	"gorm.io/gorm"
)

const watermarkName = "user_rollups"

// reconcileSQL fully recomputes aggregates for every (platform, username)
// that has new events since the watermark. The pairing CTE is the same
// algorithm as cmd/backfill-miles: the N-th login pairs with the N-th logout
// (robust to historical rows with NULL session_id), sessions over 24h are
// dropped as missed logouts, and miles = 0.1 per 3 minutes.
//
// Full recompute (not incremental addition) is what makes the tick idempotent
// and self-healing: re-running with any watermark produces the same rows.
// The aggregate subqueries deliberately aren't bounded by the captured max id
// — seeing rows newer than the watermark just makes the result more current,
// and those users get recomputed again next tick anyway.
//
// first_seen/last_seen exclude the pre-2000 sentinel: historical events rows
// carry zero-value (0001-01-01) date_created from an old insert bug.
const reconcileSQL = `
WITH dirty AS (
    SELECT DISTINCT platform, username FROM events
    WHERE id > ? AND id <= ?
),
login_rn AS (
    SELECT e.platform, e.username, e.date_created AS login_time,
           ROW_NUMBER() OVER (PARTITION BY e.platform, e.username ORDER BY e.date_created) AS rn
    FROM events e JOIN dirty d ON d.platform = e.platform AND d.username = e.username
    WHERE e.event = 'login'
),
logout_rn AS (
    SELECT e.platform, e.username, e.date_created AS logout_time,
           ROW_NUMBER() OVER (PARTITION BY e.platform, e.username ORDER BY e.date_created) AS rn
    FROM events e JOIN dirty d ON d.platform = e.platform AND d.username = e.username
    WHERE e.event = 'logout'
),
sessions AS (
    SELECT l.platform, l.username,
           EXTRACT(EPOCH FROM (lo.logout_time - l.login_time)) / 60.0 AS minutes
    FROM login_rn l
    JOIN logout_rn lo ON lo.platform = l.platform AND lo.username = l.username AND lo.rn = l.rn
    WHERE lo.logout_time > l.login_time
      AND EXTRACT(EPOCH FROM (lo.logout_time - l.login_time)) / 3600.0 < 24
),
miles AS (
    SELECT platform, username, SUM(0.1 * minutes / 3.0)::real AS events_miles
    FROM sessions GROUP BY platform, username
),
agg AS (
    SELECT d.platform, d.username,
           COALESCE(m.events_miles, 0) AS events_miles,
           (SELECT COUNT(*) FROM events e WHERE e.platform = d.platform
              AND e.username = d.username AND e.event = 'login') AS session_count,
           (SELECT MIN(e.date_created) FROM events e WHERE e.platform = d.platform
              AND e.username = d.username AND e.date_created > '2000-01-01') AS first_seen,
           (SELECT MAX(e.date_created) FROM events e WHERE e.platform = d.platform
              AND e.username = d.username AND e.date_created > '2000-01-01') AS last_seen,
           (SELECT COALESCE(SUM(e.extra_miles_earned), 0) FROM events e WHERE e.platform = d.platform
              AND e.username = d.username AND e.event IN ('logout', 'correction')) AS extra_miles
    FROM dirty d
    LEFT JOIN miles m ON m.platform = d.platform AND m.username = d.username
)
INSERT INTO user_rollups (platform, username, events_miles, session_count, first_seen, last_seen, extra_miles, date_updated)
SELECT platform, username, events_miles, session_count, first_seen, last_seen, extra_miles, now()
FROM agg
ON CONFLICT (platform, username) DO UPDATE SET
    events_miles  = EXCLUDED.events_miles,
    session_count = EXCLUDED.session_count,
    first_seen    = EXCLUDED.first_seen,
    last_seen     = EXCLUDED.last_seen,
    extra_miles   = EXCLUDED.extra_miles,
    date_updated  = now()
`

// snapshotSQL freezes a finished monthly scoreboard into scoreboard_snapshots,
// top 50 per platform. Single idempotent statement: inserts nothing if the
// board doesn't exist yet, and the NOT EXISTS guard makes the write once-only
// per board. Bots excluded, matching the live leaderboard reads.
const snapshotSQL = `
INSERT INTO scoreboard_snapshots (scoreboard_name, platform, rank, username, value)
SELECT ?, ranked.platform, ranked.rank, ranked.username, ranked.value
FROM (
    SELECT u.platform, u.username, s.value,
           ROW_NUMBER() OVER (PARTITION BY u.platform ORDER BY s.value DESC) AS rank
    FROM scores s
    JOIN scoreboards b ON b.id = s.scoreboard_id
    JOIN users u ON u.id = s.user_id
    WHERE b.name = ? AND u.is_bot = false
) ranked
WHERE ranked.rank <= 50
  AND NOT EXISTS (SELECT 1 FROM scoreboard_snapshots ss WHERE ss.scoreboard_name = ?)
`

// Reconcile is the rollup tick, registered as a background job on the twitch
// instance (it scans every platform's events regardless of which instance
// runs it). One transaction per tick: snapshot any just-finished monthly
// boards, then recompute aggregates for users with events past the watermark
// and advance it. The watermark keys on events.id — never date_created, which
// is zero-valued on historical rows.
func Reconcile(ctx context.Context, cfg *c.TripbotConfig) {
	if cfg.ReadOnly {
		return
	}

	err := database.GormDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Month-end snapshots run before the new-events check so a quiet
		// month still gets frozen on the first tick after rollover.
		for _, board := range []string{scoreboards.PreviousMilesScoreboard(), scoreboards.PreviousGuessScoreboard()} {
			res := tx.Exec(snapshotSQL, board, board, board)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected > 0 {
				slog.InfoContext(ctx, "froze monthly scoreboard snapshot",
					"scoreboard", board, "count", res.RowsAffected)
			}
		}

		// FOR UPDATE serializes overlapping ticks (belt-and-suspenders on
		// top of the job's singleton mode).
		var wm int64
		if err := tx.Raw(`SELECT last_event_id FROM rollup_watermarks WHERE name = ? FOR UPDATE`,
			watermarkName).Scan(&wm).Error; err != nil {
			return err
		}
		var maxID int64
		if err := tx.Raw(`SELECT COALESCE(MAX(id), 0) FROM events`).Scan(&maxID).Error; err != nil {
			return err
		}
		if maxID <= wm {
			return nil
		}

		res := tx.Exec(reconcileSQL, wm, maxID)
		if res.Error != nil {
			return res.Error
		}
		slog.DebugContext(ctx, "reconciled user rollups",
			"count", res.RowsAffected, "watermark", maxID)

		return tx.Exec(`UPDATE rollup_watermarks SET last_event_id = ?, date_updated = now() WHERE name = ?`,
			maxID, watermarkName).Error
	})
	if err != nil {
		slog.ErrorContext(ctx, "rollup reconcile failed", "err", err)
	}
}
