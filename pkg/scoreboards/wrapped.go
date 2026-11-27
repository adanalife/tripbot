package scoreboards

import (
	"context"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
)

// Wrapped is one viewer's month of the state-guessing game: the numbers a
// !wrapped reply is written from. Zero values mean "nothing to say" rather
// than an error — a viewer with no guesses that month gets an all-zero Wrapped.
type Wrapped struct {
	// Month is the first instant of the calendar month covered, UTC.
	Month time.Time
	// Guesses is how many answerable !guess calls the viewer made, Correct how
	// many were right, and States how many distinct states they called right.
	Guesses, Correct, States int
	// Rank is the viewer's 1-based place on the month's correct-guess board
	// (ties share a place) and Players how many are on it. Rank is 0 when the
	// viewer isn't on the board.
	Rank, Players int
	// MissedMost is the state the viewer got wrong most often and CalledIt
	// what they most often called it instead; both empty with no wrong guess.
	MissedMost, CalledIt string
	// Minutes is the viewer's closed visits in the month, summed, paired the
	// way the sessions insight pairs them.
	Minutes float64
}

// ViewerWrapped reads username's Wrapped for the month containing at, on this
// instance's platform. Four reads, each scoped to the calendar month: the guess
// log, the most-missed state, the viewer's place on the monthly board, and
// their visit minutes.
func ViewerWrapped(ctx context.Context, cfg *c.TripbotConfig, username string, at time.Time) (Wrapped, error) {
	start := time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	w := Wrapped{Month: start}
	db := database.GormDB().WithContext(ctx)

	var tally struct{ Guesses, Correct, States int }
	if err := db.Raw(`SELECT count(*) AS guesses,
       count(*) FILTER (WHERE (meta->>'correct')::boolean) AS correct,
       count(DISTINCT meta->>'actual') FILTER (WHERE (meta->>'correct')::boolean) AS states
FROM events
WHERE event = 'guess_submitted' AND platform = ? AND username = ?
  AND date_created >= ? AND date_created < ?`,
		cfg.Platform, username, start, end).Scan(&tally).Error; err != nil {
		return w, err
	}
	w.Guesses, w.Correct, w.States = tally.Guesses, tally.Correct, tally.States

	// The state missed most often, then the name it was most often given
	// instead; the ORDER BY tail makes a tie deterministic.
	var miss struct{ Actual, Guessed string }
	if err := db.Raw(`SELECT actual, guessed FROM (
    SELECT meta->>'actual' AS actual, meta->>'guessed' AS guessed, count(*) AS pair_n,
           sum(count(*)) OVER (PARTITION BY meta->>'actual') AS actual_n
    FROM events
    WHERE event = 'guess_submitted' AND platform = ? AND username = ?
      AND date_created >= ? AND date_created < ?
      AND NOT (meta->>'correct')::boolean
    GROUP BY 1, 2) m
ORDER BY actual_n DESC, pair_n DESC, actual, guessed
LIMIT 1`, cfg.Platform, username, start, end).Scan(&miss).Error; err != nil {
		return w, err
	}
	w.MissedMost, w.CalledIt = miss.Actual, miss.Guessed

	// Place on the month's board, with TopUsers' exclusions; equal scores
	// share a place rather than being split by name as the rendered board is.
	var place struct{ Players, Rank int }
	if err := db.Raw(`WITH board AS (
    SELECT u.username, s.value
    FROM scores s
    JOIN scoreboards b ON s.scoreboard_id = b.id
    JOIN users u ON s.user_id = u.id
    WHERE b.name = ? AND b.platform = ?
      AND u.is_bot = false AND u.exclude_from_leaderboard = false
      AND u.platform = ? AND u.username != ? AND s.value > 0)
SELECT count(*) AS players,
       coalesce(min(CASE WHEN username = ? THEN rk END), 0) AS rank
FROM (SELECT username, rank() OVER (ORDER BY value DESC) AS rk FROM board) r`,
		"guess_state_"+start.Format("2006_01"), cfg.Platform, cfg.Platform, cfg.ChannelName, username).Scan(&place).Error; err != nil {
		return w, err
	}
	w.Players, w.Rank = place.Players, place.Rank

	// Closed visits, paired as the sessions insight pairs them: the logout
	// sharing the login's session_id, at or after it, within 24h. Open visits
	// and the all-zero session a row without one writes are left out.
	if err := db.Raw(`SELECT coalesce(sum(EXTRACT(EPOCH FROM (o.date_created - l.date_created)) / 60.0), 0) AS minutes
FROM events l
JOIN events o ON o.session_id = l.session_id
             AND o.event = 'logout'
             AND o.date_created >= l.date_created
             AND o.date_created - l.date_created <= interval '24 hours'
WHERE l.event = 'login' AND l.platform = ? AND l.username = ?
  AND l.session_id IS NOT NULL
  AND l.session_id <> '00000000-0000-0000-0000-000000000000'
  AND l.date_created >= ? AND l.date_created < ?`,
		cfg.Platform, username, start, end).Scan(&w.Minutes).Error; err != nil {
		return w, err
	}
	return w, nil
}
