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
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	"github.com/adanalife/tripbot/pkg/scoreboards"
	"gorm.io/gorm"
)

const watermarkName = "user_rollups"

// reconcileSQL fully recomputes aggregates for every (platform, username)
// that has new events since the watermark. A login pairs with the logout that
// carries its session_id, so a login that never got a logout (a crash) pairs
// with nothing rather than shifting every later pair. Legacy rows from before
// session_id existed (NULL, or the zero UUID) fall back to the
// cmd/backfill-miles algorithm: the N-th legacy login pairs with the N-th
// legacy logout. Sessions over 24h are dropped as missed logouts, and miles =
// 0.1 per 3 minutes.
//
// Full recompute (not incremental addition) is what makes the tick idempotent
// and self-healing: re-running with any watermark produces the same rows.
// The aggregate subqueries deliberately aren't bounded by the captured max id
// — seeing rows newer than the watermark just makes the result more current,
// and those users get recomputed again next tick anyway.
//
// first_seen/last_seen sit behind a pre-2000 floor, so an events row left with
// an unstamped (0001-01-01) date_created can't drag a user's first_seen back to
// year zero.
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
      AND COALESCE(e.session_id, '00000000-0000-0000-0000-000000000000') = '00000000-0000-0000-0000-000000000000'
),
logout_rn AS (
    SELECT e.platform, e.username, e.date_created AS logout_time,
           ROW_NUMBER() OVER (PARTITION BY e.platform, e.username ORDER BY e.date_created) AS rn
    FROM events e JOIN dirty d ON d.platform = e.platform AND d.username = e.username
    WHERE e.event = 'logout'
      AND COALESCE(e.session_id, '00000000-0000-0000-0000-000000000000') = '00000000-0000-0000-0000-000000000000'
),
-- MIN collapses a session_id that was logged out twice into one session.
keyed AS (
    SELECT e.platform, e.username, e.session_id,
           MIN(e.date_created) FILTER (WHERE e.event = 'login') AS login_time,
           MIN(e.date_created) FILTER (WHERE e.event = 'logout') AS logout_time
    FROM events e JOIN dirty d ON d.platform = e.platform AND d.username = e.username
    WHERE e.event IN ('login', 'logout')
      AND e.session_id <> '00000000-0000-0000-0000-000000000000'
    GROUP BY e.platform, e.username, e.session_id
),
pairs AS (
    SELECT l.platform, l.username, l.login_time, lo.logout_time
    FROM login_rn l
    JOIN logout_rn lo ON lo.platform = l.platform AND lo.username = l.username AND lo.rn = l.rn
    UNION ALL
    SELECT platform, username, login_time, logout_time FROM keyed
),
sessions AS (
    SELECT platform, username,
           EXTRACT(EPOCH FROM (logout_time - login_time)) / 60.0 AS minutes
    FROM pairs
    WHERE logout_time > login_time
      AND EXTRACT(EPOCH FROM (logout_time - login_time)) / 3600.0 < 24
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
// per board. Bots and opted-out accounts excluded, matching the live
// leaderboard reads.
const snapshotSQL = `
INSERT INTO scoreboard_snapshots (scoreboard_name, platform, rank, username, value)
SELECT ?, ranked.platform, ranked.rank, ranked.username, ranked.value
FROM (
    SELECT u.platform, u.username, s.value,
           ROW_NUMBER() OVER (PARTITION BY u.platform ORDER BY s.value DESC) AS rank
    FROM scores s
    JOIN scoreboards b ON b.id = s.scoreboard_id
    JOIN users u ON u.id = s.user_id
    WHERE b.name = ? AND u.is_bot = false AND u.exclude_from_leaderboard = false
) ranked
WHERE ranked.rank <= 50
  AND NOT EXISTS (SELECT 1 FROM scoreboard_snapshots ss WHERE ss.scoreboard_name = ?)
`

// dailySnapshotSQL records the day's first look at a live monthly board into
// scoreboard_daily_snapshots, top 50 per platform — the baseline the
// leaderboards measure "since the start of the day" against. The same
// idempotent shape as snapshotSQL: nothing if the board has no scores yet, and
// the NOT EXISTS guard makes it once-only per board per day. b.platform =
// u.platform matches the live read (scoreboards.TopUsers), so a legacy score
// hanging off the other platform's same-named board can't put a viewer on the
// board twice.
const dailySnapshotSQL = `
INSERT INTO scoreboard_daily_snapshots (day, scoreboard_name, platform, rank, username, value)
SELECT ?, ?, ranked.platform, ranked.rank, ranked.username, ranked.value
FROM (
    SELECT u.platform, u.username, s.value,
           ROW_NUMBER() OVER (PARTITION BY u.platform ORDER BY s.value DESC, u.username ASC) AS rank
    FROM scores s
    JOIN scoreboards b ON b.id = s.scoreboard_id
    JOIN users u ON u.id = s.user_id
    WHERE b.name = ? AND b.platform = u.platform
      AND u.is_bot = false AND u.exclude_from_leaderboard = false
) ranked
WHERE ranked.rank <= 50
  AND NOT EXISTS (SELECT 1 FROM scoreboard_daily_snapshots ds WHERE ds.day = ? AND ds.scoreboard_name = ?)
`

// dailySnapshotRetention is how long a daily snapshot is kept. Only today's is
// read; the rest is a short history for looking back at by hand.
const dailySnapshotRetention = 40 * 24 * time.Hour

// Reconcile is the rollup tick, registered as a background job on the twitch
// instance (it scans every platform's events regardless of which instance
// runs it). One transaction per tick: snapshot any just-finished monthly
// boards, take the day's first snapshot of the live ones (pruning old days),
// then recompute aggregates for users with events past the watermark
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

		// The day is named on the same clock the leaderboards handler names
		// the current month with, so the two agree on what "today" is.
		now := time.Now()
		day := now.Format("2006-01-02")
		for _, board := range []string{scoreboards.CurrentMilesScoreboard(), scoreboards.CurrentGuessScoreboard()} {
			res := tx.Exec(dailySnapshotSQL, day, board, board, day, board)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected > 0 {
				slog.InfoContext(ctx, "took daily scoreboard snapshot",
					"scoreboard", board, "count", res.RowsAffected)
			}
		}
		if err := tx.Exec(`DELETE FROM scoreboard_daily_snapshots WHERE day < ?`,
			now.Add(-dailySnapshotRetention).Format("2006-01-02")).Error; err != nil {
			return err
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
