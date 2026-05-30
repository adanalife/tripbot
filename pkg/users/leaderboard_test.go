package users

import (
	"context"
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
			got := strToFloat32(context.Background(), tt.in)
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
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
	insertIntoLeaderboard(context.Background(), u)

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
	insertIntoLeaderboard(context.Background(), u)

	if len(LifetimeMilesLeaderboard) != 2 {
		t.Fatalf("expected length unchanged after replace, got %d: %v",
			len(LifetimeMilesLeaderboard), LifetimeMilesLeaderboard)
	}
	if LifetimeMilesLeaderboard[0][0] != "bob" {
		t.Fatalf("expected bob to be #1 with 200 miles, got %v", LifetimeMilesLeaderboard)
	}
}
