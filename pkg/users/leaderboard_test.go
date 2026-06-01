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

func TestRemoveFromLeaderboard(t *testing.T) {
	s := New(noopChatterSource{})
	s.lifetimeLeaderboard = [][]string{
		{"alice", "100"}, {"bob", "75"}, {"carol", "50"},
	}

	s.removeFromLeaderboard("bob")

	if len(s.lifetimeLeaderboard) != 2 {
		t.Fatalf("expected 2 entries after remove, got %d", len(s.lifetimeLeaderboard))
	}
	for _, pair := range s.lifetimeLeaderboard {
		if pair[0] == "bob" {
			t.Fatal("bob still present after remove")
		}
	}
}

func TestRemoveFromLeaderboardMissing(t *testing.T) {
	s := New(noopChatterSource{})
	s.lifetimeLeaderboard = [][]string{
		{"alice", "100"}, {"bob", "75"},
	}

	s.removeFromLeaderboard("nobody")

	if len(s.lifetimeLeaderboard) != 2 {
		t.Fatalf("expected unchanged length after missing remove, got %d", len(s.lifetimeLeaderboard))
	}
}

func TestRemoveFromLeaderboardEmpty(t *testing.T) {
	s := New(noopChatterSource{})

	s.removeFromLeaderboard("alice")

	if len(s.lifetimeLeaderboard) != 0 {
		t.Fatalf("expected empty leaderboard, got %v", s.lifetimeLeaderboard)
	}
}

// insertIntoLeaderboard pulls miles from User.CurrentMiles. The user isn't in
// the default session, so CurrentMiles returns User.Miles directly (no session
// bonus) — making the math deterministic.
func TestInsertIntoLeaderboardOrdersByMilesDesc(t *testing.T) {
	s := New(noopChatterSource{})
	s.lifetimeLeaderboard = [][]string{
		{"existing-1", "100"},
		{"existing-2", "50"},
		{"existing-3", "10"},
	}

	u := User{Username: "newcomer", Miles: 75}
	s.insertIntoLeaderboard(context.Background(), u)

	if len(s.lifetimeLeaderboard) != 4 {
		t.Fatalf("expected 4 entries after insert, got %d: %v",
			len(s.lifetimeLeaderboard), s.lifetimeLeaderboard)
	}
	if s.lifetimeLeaderboard[1][0] != "newcomer" {
		t.Fatalf("expected newcomer at index 1 (between 100 and 50), got order %v", s.lifetimeLeaderboard)
	}
}

func TestInsertIntoLeaderboardReplacesExistingUser(t *testing.T) {
	s := New(noopChatterSource{})
	s.lifetimeLeaderboard = [][]string{
		{"alice", "100"},
		{"bob", "50"},
	}

	// Bob's miles increased to 200 — should jump above alice and replace his old row.
	u := User{Username: "bob", Miles: 200}
	s.insertIntoLeaderboard(context.Background(), u)

	if len(s.lifetimeLeaderboard) != 2 {
		t.Fatalf("expected length unchanged after replace, got %d: %v",
			len(s.lifetimeLeaderboard), s.lifetimeLeaderboard)
	}
	if s.lifetimeLeaderboard[0][0] != "bob" {
		t.Fatalf("expected bob to be #1 with 200 miles, got %v", s.lifetimeLeaderboard)
	}
}
