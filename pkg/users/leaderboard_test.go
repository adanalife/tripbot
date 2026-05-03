package users

import (
	"strings"
	"testing"
)

func TestStrToFloat32Valid(t *testing.T) {
	tests := []struct {
		in   string
		want float32
	}{
		{"0", 0},
		{"1.5", 1.5},
		{"42.0", 42.0},
		{"-3.25", -3.25},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := strToFloat32(tt.in)
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLeaderboardContent(t *testing.T) {
	board := [][]string{
		{"alice", "100.5"},
		{"bob", "75.2"},
		{"carol", "50.0"},
	}
	got := LeaderboardContent("monthly miles", board)

	if !strings.HasPrefix(got, "Monthly Miles\n") {
		t.Fatalf("expected title-cased header, got %q", got)
	}
	for _, name := range []string{"alice", "bob", "carol"} {
		if !strings.Contains(got, name) {
			t.Fatalf("expected %q in output, got %q", name, got)
		}
	}
	if !strings.Contains(got, "100.5 (alice)") {
		t.Fatalf("expected '100.5 (alice)' format, got %q", got)
	}
}

func TestLeaderboardContentTruncatesToFive(t *testing.T) {
	board := [][]string{
		{"u1", "10"}, {"u2", "9"}, {"u3", "8"}, {"u4", "7"},
		{"u5", "6"}, {"u6", "5"}, {"u7", "4"},
	}
	got := LeaderboardContent("top", board)

	if strings.Contains(got, "u6") || strings.Contains(got, "u7") {
		t.Fatalf("expected truncation to 5 entries, got %q", got)
	}
	for _, name := range []string{"u1", "u2", "u3", "u4", "u5"} {
		if !strings.Contains(got, name) {
			t.Fatalf("expected %q in top-5, got %q", name, got)
		}
	}
}

func TestLeaderboardContentSmallerThanFive(t *testing.T) {
	board := [][]string{
		{"alice", "100"},
		{"bob", "50"},
	}
	got := LeaderboardContent("tiny", board)
	if !strings.Contains(got, "alice") || !strings.Contains(got, "bob") {
		t.Fatalf("expected both names, got %q", got)
	}
}

func TestLeaderboardContentEmpty(t *testing.T) {
	got := LeaderboardContent("nobody", nil)
	if got != "Nobody\n" {
		t.Fatalf("got %q, want %q", got, "Nobody\n")
	}
}

// snapshotLeaderboard saves the global LifetimeMilesLeaderboard and returns a
// restore func. Use with defer to keep tests from contaminating each other.
func snapshotLeaderboard() func() {
	saved := make([][]string, len(LifetimeMilesLeaderboard))
	copy(saved, LifetimeMilesLeaderboard)
	return func() { LifetimeMilesLeaderboard = saved }
}

func TestRemoveFromLeaderboard(t *testing.T) {
	defer snapshotLeaderboard()()
	LifetimeMilesLeaderboard = [][]string{
		{"alice", "100"}, {"bob", "75"}, {"carol", "50"},
	}

	removeFromLeaderboard("bob")

	if len(LifetimeMilesLeaderboard) != 2 {
		t.Fatalf("expected 2 entries after remove, got %d", len(LifetimeMilesLeaderboard))
	}
	for _, pair := range LifetimeMilesLeaderboard {
		if pair[0] == "bob" {
			t.Fatal("bob still present after remove")
		}
	}
}

func TestRemoveFromLeaderboardMissing(t *testing.T) {
	defer snapshotLeaderboard()()
	LifetimeMilesLeaderboard = [][]string{
		{"alice", "100"}, {"bob", "75"},
	}

	removeFromLeaderboard("nobody")

	if len(LifetimeMilesLeaderboard) != 2 {
		t.Fatalf("expected unchanged length after missing remove, got %d", len(LifetimeMilesLeaderboard))
	}
}

func TestRemoveFromLeaderboardEmpty(t *testing.T) {
	defer snapshotLeaderboard()()
	LifetimeMilesLeaderboard = nil

	removeFromLeaderboard("alice")

	if len(LifetimeMilesLeaderboard) != 0 {
		t.Fatalf("expected empty leaderboard, got %v", LifetimeMilesLeaderboard)
	}
}

// insertIntoLeaderboard pulls miles from User.CurrentMiles, which uses the
// global LoggedIn map. We seed the map with a session snapshot so the math is
// deterministic.
func TestInsertIntoLeaderboardOrdersByMilesDesc(t *testing.T) {
	defer snapshotLeaderboard()()
	LifetimeMilesLeaderboard = [][]string{
		{"existing-1", "100"},
		{"existing-2", "50"},
		{"existing-3", "10"},
	}

	savedLoggedIn := LoggedIn
	defer func() { LoggedIn = savedLoggedIn }()
	LoggedIn = map[string]*User{}

	// Miles=75 from User.Miles directly (no session bonus, since not in LoggedIn).
	u := User{Username: "newcomer", Miles: 75}
	insertIntoLeaderboard(u)

	if len(LifetimeMilesLeaderboard) != 4 {
		t.Fatalf("expected 4 entries after insert, got %d: %v",
			len(LifetimeMilesLeaderboard), LifetimeMilesLeaderboard)
	}
	if LifetimeMilesLeaderboard[1][0] != "newcomer" {
		t.Fatalf("expected newcomer at index 1 (between 100 and 50), got order %v", LifetimeMilesLeaderboard)
	}
}

func TestInsertIntoLeaderboardReplacesExistingUser(t *testing.T) {
	defer snapshotLeaderboard()()
	LifetimeMilesLeaderboard = [][]string{
		{"alice", "100"},
		{"bob", "50"},
	}

	savedLoggedIn := LoggedIn
	defer func() { LoggedIn = savedLoggedIn }()
	LoggedIn = map[string]*User{}

	// Bob's miles increased to 200 — should jump above alice and replace his old row.
	u := User{Username: "bob", Miles: 200}
	insertIntoLeaderboard(u)

	if len(LifetimeMilesLeaderboard) != 2 {
		t.Fatalf("expected length unchanged after replace, got %d: %v",
			len(LifetimeMilesLeaderboard), LifetimeMilesLeaderboard)
	}
	if LifetimeMilesLeaderboard[0][0] != "bob" {
		t.Fatalf("expected bob to be #1 with 200 miles, got %v", LifetimeMilesLeaderboard)
	}
}
