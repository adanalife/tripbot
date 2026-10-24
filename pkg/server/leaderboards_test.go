package server

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/database/testdb"
	"gorm.io/gorm"
)

// A month the list doesn't hold has to read as the current month rather than
// as an empty board: the console builds its selector from the same Months
// list, so the only way to ask for one is with a stale client, and an empty
// pane reads as "nobody scored" rather than as "that month isn't there".
func TestSelectMonth(t *testing.T) {
	months := []string{"2026-09", "2026-08", "2026-07"}
	for _, tt := range []struct {
		name, asked, want string
	}{
		{"a month with data is served", "2026-08", "2026-08"},
		{"no month asked", "", "2026-09"},
		{"a month with no data", "2025-01", "2026-09"},
		{"underscore spelling is not the API's", "2026_08", "2026-09"},
		{"month 13", "2026-13", "2026-09"},
		{"month 00", "2026-00", "2026-09"},
		{"trailing junk", "2026-08x", "2026-09"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := selectMonth(tt.asked, months, "2026-09"); got != tt.want {
				t.Errorf("selectMonth(%q) = %q, want %q", tt.asked, got, tt.want)
			}
		})
	}
}

// The month in progress has no snapshot by definition, so it is never in the
// query's answer — and on the first of a month, before the freeze has run, it
// is the only month there is.
func TestWithCurrentMonth(t *testing.T) {
	for _, tt := range []struct {
		name    string
		months  []string
		current string
		want    []string
	}{
		{"prepended when absent", []string{"2026-08"}, "2026-09", []string{"2026-09", "2026-08"}},
		{"no snapshots yet", nil, "2026-09", []string{"2026-09"}},
		{
			// A month that has both live scores and a snapshot would list
			// twice, and the selector would show the same month twice.
			name:    "not duplicated when already present",
			months:  []string{"2026-09", "2026-08"},
			current: "2026-09",
			want:    []string{"2026-09", "2026-08"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := withCurrentMonth(tt.months, tt.current); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("withCurrentMonth(%v, %q) = %v, want %v", tt.months, tt.current, got, tt.want)
			}
		})
	}
}

// Ranks are shared across a tie and the next place skips, matching what the
// overlay and the Discord embed print for the same rows — the point of ranking
// server-side is that all three agree.
func TestRankedBoard(t *testing.T) {
	got := rankedBoard("miles", "Miles", [][]string{
		{"viewer1", "9.0"}, {"viewer2", "4.0"}, {"viewer3", "4.0"}, {"viewer4", "1.0"},
	})
	if got.Board != "miles" || got.Label != "Miles" {
		t.Errorf("board identity = %q/%q", got.Board, got.Label)
	}
	want := []leaderboardRank{
		{Rank: 1, Username: "viewer1", Value: "9.0"},
		{Rank: 2, Username: "viewer2", Value: "4.0"},
		{Rank: 2, Username: "viewer3", Value: "4.0"},
		{Rank: 4, Username: "viewer4", Value: "1.0"},
	}
	if !reflect.DeepEqual(got.Rows, want) {
		t.Errorf("rows = %+v, want %+v", got.Rows, want)
	}
}

// An empty board has to encode as [] rather than null: the console renders the
// rows array directly, and a null is a crash rather than an empty pane.
func TestRankedBoardEmpty(t *testing.T) {
	got := rankedBoard("guesses", "Correct Guesses", nil)
	if got.Rows == nil {
		t.Fatal("rows is nil, want an empty slice so it encodes as []")
	}
	if len(got.Rows) != 0 {
		t.Errorf("rows = %+v, want empty", got.Rows)
	}
}

func intp(n int) *int { return &n }

// A previous rank is ranked the way the live one is, so a tie in the snapshot
// shares a place; a viewer the snapshot doesn't hold is 0 ("new today"); and
// with no snapshot at all, nothing is filled.
func TestWithPrevRanks(t *testing.T) {
	live := func() leaderboardBoard {
		return rankedBoard("miles", "Miles", [][]string{
			{"climber", "9.0"}, {"tied_a", "5.0"}, {"tied_b", "4.0"}, {"newcomer", "1.0"},
		})
	}
	snapshot := [][]string{{"leader", "8.0"}, {"tied_a", "3.0"}, {"tied_b", "3.0"}, {"climber", "2.0"}}

	board := live()
	withPrevRanks(&board, snapshot, true)
	want := map[string]*int{"climber": intp(4), "tied_a": intp(2), "tied_b": intp(2), "newcomer": intp(0)}
	for _, row := range board.Rows {
		if row.PrevRank == nil || *row.PrevRank != *want[row.Username] {
			t.Errorf("%s prev_rank = %v, want %d", row.Username, row.PrevRank, *want[row.Username])
		}
	}

	board = live()
	withPrevRanks(&board, nil, false)
	for _, row := range board.Rows {
		if row.PrevRank != nil {
			t.Errorf("%s prev_rank = %d with no snapshot, want nil", row.Username, *row.PrevRank)
		}
	}
}

// seedLiveScore writes a twitch viewer's score on a live monthly board,
// creating the board on first use.
func seedLiveScore(t *testing.T, db *gorm.DB, board, username string, value float64) {
	t.Helper()
	err := db.Exec(`INSERT INTO scoreboards (name, platform) SELECT ?, 'twitch'
	                WHERE NOT EXISTS (SELECT 1 FROM scoreboards WHERE name = ? AND platform = 'twitch')`,
		board, board).Error
	if err != nil {
		t.Fatalf("insert scoreboard %s: %v", board, err)
	}
	err = db.Exec(`INSERT INTO users (username, platform) VALUES (?, 'twitch')
	               ON CONFLICT DO NOTHING`, username).Error
	if err != nil {
		t.Fatalf("insert user %s: %v", username, err)
	}
	err = db.Exec(`INSERT INTO scores (user_id, scoreboard_id, value)
	               SELECT u.id, b.id, ? FROM users u, scoreboards b
	               WHERE u.username = ? AND u.platform = 'twitch' AND b.name = ? AND b.platform = 'twitch'`,
		value, username, board).Error
	if err != nil {
		t.Fatalf("insert score for %s: %v", username, err)
	}
}

func seedDailySnapshot(t *testing.T, db *gorm.DB, day, board string, rank int, username string, value float64) {
	t.Helper()
	err := db.Exec(`INSERT INTO scoreboard_daily_snapshots (day, scoreboard_name, platform, rank, username, value)
	                VALUES (?, ?, 'twitch', ?, ?, ?)`, day, board, rank, username, value).Error
	if err != nil {
		t.Fatalf("insert daily snapshot %s/%s: %v", board, username, err)
	}
}

func getLeaderboards(t *testing.T, path string) (leaderboardsResponse, string) {
	t.Helper()
	rec := insightsGET(t, path, communityServer.leaderboardsHandler)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got leaderboardsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}
	return got, rec.Body.String()
}

func boardPrevRanks(got leaderboardsResponse, board string) map[string]*int {
	out := map[string]*int{}
	for _, b := range got.Boards {
		if b.Board == board {
			for _, row := range b.Rows {
				out[row.Username] = row.PrevRank
			}
		}
	}
	return out
}

// The live month carries each row's place at the start of today: N from the
// day's snapshot (ties shared), 0 for a viewer the snapshot doesn't hold.
func TestLeaderboardsHandler_PrevRankFromTodaysSnapshot(t *testing.T) {
	db := testdb.New(t)
	now := time.Now()
	miles := "miles_" + now.Format("2006_01")
	guesses := "guess_state_" + now.Format("2006_01")
	today := now.Format("2006-01-02")

	seedLiveScore(t, db, miles, "lb_climber", 9)
	seedLiveScore(t, db, miles, "lb_tied_a", 5)
	seedLiveScore(t, db, miles, "lb_newcomer", 1)
	seedDailySnapshot(t, db, today, miles, 1, "lb_leader", 8)
	seedDailySnapshot(t, db, today, miles, 2, "lb_tied_a", 3)
	seedDailySnapshot(t, db, today, miles, 3, "lb_tied_b", 3)
	seedDailySnapshot(t, db, today, miles, 4, "lb_climber", 2)
	// Yesterday's snapshot is not today's baseline.
	seedDailySnapshot(t, db, now.AddDate(0, 0, -1).Format("2006-01-02"), miles, 1, "lb_newcomer", 50)

	// The guess board has live scores but no snapshot today.
	seedLiveScore(t, db, guesses, "lb_guesser", 3)

	got, _ := getLeaderboards(t, "/api/leaderboards")
	if !got.Live {
		t.Fatalf("live = false for the current month %q", got.Month)
	}
	prev := boardPrevRanks(got, "miles")
	for user, want := range map[string]int{"lb_climber": 4, "lb_tied_a": 2, "lb_newcomer": 0} {
		if prev[user] == nil || *prev[user] != want {
			t.Errorf("miles %s prev_rank = %v, want %d", user, prev[user], want)
		}
	}
	guessPrev := boardPrevRanks(got, "guesses")
	if p, ok := guessPrev["lb_guesser"]; !ok || p != nil {
		t.Errorf("guesses lb_guesser prev_rank = %v (present %v), want omitted with no snapshot today", p, ok)
	}
}

// A frozen month never moves, so it carries no previous ranks at all — the
// field is omitted, not 0, which the console would draw as "new".
func TestLeaderboardsHandler_FrozenMonthOmitsPrevRank(t *testing.T) {
	db := testdb.New(t)
	seedSnapshot(t, db, "miles_2025_03", 1, "lb_frozen", 40)
	seedDailySnapshot(t, db, time.Now().Format("2006-01-02"), "miles_2025_03", 1, "lb_frozen", 40)

	got, body := getLeaderboards(t, "/api/leaderboards?month=2025-03")
	if got.Live || got.Month != "2025-03" {
		t.Fatalf("month = %q live = %v, want the frozen 2025-03", got.Month, got.Live)
	}
	if _, ok := boardPrevRanks(got, "miles")["lb_frozen"]; !ok {
		t.Fatalf("frozen board lost its row: %s", body)
	}
	if strings.Contains(body, "prev_rank") {
		t.Errorf("frozen month carries prev_rank: %s", body)
	}
}
