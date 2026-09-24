package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/adanalife/tripbot/pkg/database"
	"github.com/adanalife/tripbot/pkg/scoreboards"
)

// The leaderboards endpoint serves the monthly boards the console's guessr
// pane renders beside the guessr board, for any month the project has data
// for. Two sources back it, and which one answers is decided by the month
// asked for rather than by a flag: the current month is still accumulating, so
// it reads the live scores table, and a finished month reads the frozen
// snapshot the rollup tick wrote at rollover. The all-time miles board is not
// here — /api/stats/community already serves it.
//
// Boards are per-platform, like every other scoreboard read: the instance
// answers for its own cfg.Platform.

// leaderboardSize is how many placements a board returns. The snapshot keeps
// 50; a console pane shows a screenful, and the console can't ask for more
// because nothing has wanted to.
const leaderboardSize = 25

// monthParam is the ?month= shape, matching the YYYY-MM the payload hands out
// rather than the YYYY_MM the board names embed — the underscore is an
// internal spelling and the API shouldn't leak it.
var monthParam = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)

// leaderboardBoard is one board for one month. Rows are ordered best-first and
// Ranks[i] is Rows[i]'s place, sharing the place across a tie.
type leaderboardBoard struct {
	Board string            `json:"board"`
	Label string            `json:"label"`
	Rows  []leaderboardRank `json:"rows"`
}

type leaderboardRank struct {
	Rank     int    `json:"rank"`
	Username string `json:"username"`
	// Value stays a string: it is the rendered score every other leaderboard
	// surface prints, one decimal for miles and a whole number for guesses,
	// and re-parsing it client-side is how two surfaces start disagreeing
	// about a tie.
	Value string `json:"value"`
}

type leaderboardsResponse struct {
	// Month is the month served, "YYYY-MM".
	Month string `json:"month"`
	// Live is true when Month is the month in progress, i.e. the numbers can
	// still move. A frozen month never changes again.
	Live bool `json:"live"`
	// Months lists every month that can be asked for, newest first, so the
	// console builds its selector from this one call.
	Months []string           `json:"months"`
	Boards []leaderboardBoard `json:"boards"`
}

// leaderboardsHandler serves GET /api/leaderboards[?month=YYYY-MM]: the miles
// and guess boards for one month, plus the list of months that have data.
// An unparseable or unknown month falls back to the current one rather than
// erroring — the selector is built from this endpoint's own Months list, so a
// bad value means a stale client, not a request worth failing.
func (s *Server) leaderboardsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	current := time.Now().Format("2006-01")

	months, err := snapshotMonths(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "leaderboard months query failed", "err", err)
		insightsError(w, "couldn't list leaderboard months")
		return
	}
	months = withCurrentMonth(months, current)

	month := selectMonth(r.URL.Query().Get("month"), months, current)

	live := month == current
	suffix := month[:4] + "_" + month[5:]
	topUsers := scoreboards.SnapshotTopUsers
	if live {
		topUsers = scoreboards.TopUsers
	}

	payload := leaderboardsResponse{
		Month:  month,
		Live:   live,
		Months: months,
		Boards: []leaderboardBoard{
			rankedBoard("miles", "Miles", topUsers(ctx, s.cfg, "miles_"+suffix, leaderboardSize)),
			rankedBoard("guesses", "Correct Guesses",
				scoreboards.GuessRows(topUsers(ctx, s.cfg, "guess_state_"+suffix, leaderboardSize))),
		},
	}
	writeInsights(w, r, payload)
}

// selectMonth resolves ?month= against the months that actually have data,
// falling back to current. Anything the list doesn't hold — a malformed value,
// a month before the project existed, a month that hasn't been frozen yet —
// comes back as current: the selector is built from the same Months list this
// checks against, so an unknown value means a stale client rather than a
// request worth failing.
func selectMonth(asked string, months []string, current string) string {
	if !monthParam.MatchString(asked) {
		return current
	}
	for _, m := range months {
		if m == asked {
			return asked
		}
	}
	return current
}

// rankedBoard turns the [username, value] pairs every scoreboards read returns
// into the ranked rows the API serves, so a tie reads the same place here as
// it does on the overlay and in Discord.
func rankedBoard(name, label string, pairs [][]string) leaderboardBoard {
	board := leaderboardBoard{Board: name, Label: label, Rows: []leaderboardRank{}}
	ranks := scoreboards.Ranks(pairs)
	for i, pair := range pairs {
		if len(pair) < 2 {
			continue
		}
		board.Rows = append(board.Rows,
			leaderboardRank{Rank: ranks[i], Username: pair[0], Value: pair[1]})
	}
	return board
}

// snapshotMonthsSQL lists the months scoreboard_snapshots holds, newest first.
// The month is the _YYYY_MM suffix on the board name, which is the identity
// the boards are queried by everywhere else — date_created would agree today
// but describes when the freeze ran, not what it froze.
const snapshotMonthsSQL = `
SELECT DISTINCT substring(scoreboard_name FROM '[0-9]{4}_[0-9]{2}$') AS month
FROM scoreboard_snapshots
WHERE substring(scoreboard_name FROM '[0-9]{4}_[0-9]{2}$') IS NOT NULL
ORDER BY month DESC`

func snapshotMonths(ctx context.Context) ([]string, error) {
	var raw []struct{ Month string }
	if err := database.GormDB().WithContext(ctx).Raw(snapshotMonthsSQL).Scan(&raw).Error; err != nil {
		return nil, fmt.Errorf("snapshot months: %w", err)
	}
	months := make([]string, 0, len(raw))
	for _, r := range raw {
		// The SQL can only yield YYYY_MM, but a board name is free text in the
		// table; a shorter match would slice out of range rather than fail.
		if len(r.Month) != len("2006_01") {
			continue
		}
		months = append(months, r.Month[:4]+"-"+r.Month[5:])
	}
	return months, nil
}

// withCurrentMonth puts the month in progress at the front of the list. It has
// no snapshot by definition, so it is never in the query's answer — and on the
// first of a month, before the previous month's freeze has run, it is the only
// month there is.
func withCurrentMonth(months []string, current string) []string {
	for _, m := range months {
		if m == current {
			return months
		}
	}
	return append([]string{current}, months...)
}
