package users

import (
	"context"
	"strings"
	"testing"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database/testdb"
	"gorm.io/gorm"
)

// seedUsers inserts users rows through GORM, so the model's column mapping is
// the same one the code under test reads back through.
func seedUsers(t *testing.T, db *gorm.DB, users ...User) {
	t.Helper()
	for i := range users {
		if users[i].Platform == "" {
			users[i].Platform = c.Conf.Platform
		}
		if err := db.Create(&users[i]).Error; err != nil {
			t.Fatalf("seeding user %q: %v", users[i].Username, err)
		}
	}
}

// assertBoard compares a leaderboard against the expected [username, miles] pairs.
func assertBoard(t *testing.T, got [][]string, want [][]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %d entries, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i][0] != want[i][0] || got[i][1] != want[i][1] {
			t.Fatalf("row %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

// The fetch must rank by stored miles, scoped to this instance's platform, and
// exclude bots, the channel owner, and users with no miles.
func TestUpdateLeaderboard_ReadsFromDB(t *testing.T) {
	db := testdb.New(t)
	seedUsers(t, db,
		User{Username: "alice", Miles: 100},
		User{Username: "bob", Miles: 50},
		User{Username: "carol", Miles: 75},
		User{Username: "botty", Miles: 200, IsBot: true},
		User{Username: strings.ToLower(c.Conf.ChannelName), Miles: 300},
		User{Username: "zed", Miles: 0},
		User{Username: "elsewhere", Miles: 500, Platform: "youtube"},
	)

	s := New(noopChatterSource{})
	s.UpdateLeaderboard(context.Background())

	assertBoard(t, s.LifetimeLeaderboard(), [][]string{
		{"alice", "100.0"},
		{"carol", "75.0"},
		{"bob", "50.0"},
	})
}

// InitLeaderboard hydrates the same board at boot.
func TestInitLeaderboard_ReadsFromDB(t *testing.T) {
	db := testdb.New(t)
	seedUsers(t, db,
		User{Username: "alice", Miles: 10},
		User{Username: "bob", Miles: 20},
	)

	s := New(noopChatterSource{})
	s.InitLeaderboard(context.Background())

	assertBoard(t, s.LifetimeLeaderboard(), [][]string{
		{"bob", "20.0"},
		{"alice", "10.0"},
	})
}

// A logged-in user's in-progress session miles overlay their stored miles and
// can reorder the board between logouts.
func TestUpdateLeaderboard_OverlaysLiveSessionMiles(t *testing.T) {
	db := testdb.New(t)
	seedUsers(t, db,
		User{Username: "alice", Miles: 100},
		User{Username: "bob", Miles: 99},
	)

	s := New(noopChatterSource{})
	// bob has been logged in for ~10 hours: 0.1mi/3min = ~20 miles in
	// progress, enough to pass alice before his logout writes it back.
	s.loggedIn["bob"] = &User{Username: "bob", Miles: 99, LoggedIn: time.Now().Add(-10 * time.Hour)}
	s.UpdateLeaderboard(context.Background())

	got := s.LifetimeLeaderboard()
	if len(got) != 2 || got[0][0] != "bob" {
		t.Fatalf("expected bob first after live-miles overlay, got %v", got)
	}
}

// A failed fetch must leave the previous board in place rather than blanking
// what the rotators display.
func TestUpdateLeaderboard_KeepsCacheOnError(t *testing.T) {
	testdb.New(t)

	s := New(noopChatterSource{})
	s.lifetimeLeaderboard = [][]string{{"alice", "100.0"}}

	// A cancelled context is the cheapest real query failure.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.UpdateLeaderboard(ctx)

	assertBoard(t, s.LifetimeLeaderboard(), [][]string{{"alice", "100.0"}})
}
