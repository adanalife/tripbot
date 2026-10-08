package server

import (
	"reflect"
	"testing"
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
