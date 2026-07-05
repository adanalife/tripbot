package users

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// installMockDB mirrors pkg/chatbot's helper: a sqlmock-backed *gorm.DB
// installed as the process-wide singleton so fetchLeaderboard routes to it.
func installMockDB(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	database.SetGormDB(gdb)
	t.Cleanup(func() {
		database.SetGormDB(nil)
		_ = sqlDB.Close()
	})
	return mock
}

func leaderboardRows(rows ...[]driver.Value) *sqlmock.Rows {
	r := sqlmock.NewRows([]string{"username", "miles", "platform", "is_bot"})
	for _, row := range rows {
		r.AddRow(row...)
	}
	return r
}

// TestUpdateLeaderboard_ReadsFromDB pins the platform-scoping contract with
// strict args: the fetch must filter by this instance's platform and exclude
// the channel owner (loose regexes have silently absorbed missing platform
// filters before).
func TestUpdateLeaderboard_ReadsFromDB(t *testing.T) {
	mock := installMockDB(t)
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE platform = \$1 AND miles != 0 AND is_bot = false AND username != \$2 ORDER BY miles DESC LIMIT \$3`).
		WithArgs(c.Conf.Platform, strings.ToLower(c.Conf.ChannelName), maxLeaderboardSize).
		WillReturnRows(leaderboardRows(
			[]driver.Value{"alice", float32(100), "twitch", false},
			[]driver.Value{"bob", float32(50), "twitch", false},
		))

	s := New(noopChatterSource{})
	s.UpdateLeaderboard(context.Background())

	want := [][]string{{"alice", "100.0"}, {"bob", "50.0"}}
	got := s.LifetimeLeaderboard()
	if len(got) != len(want) {
		t.Fatalf("expected %d entries, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i][0] != want[i][0] || got[i][1] != want[i][1] {
			t.Fatalf("row %d: got %v, want %v", i, got[i], want[i])
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// A logged-in user's in-progress session miles overlay their stored miles and
// can reorder the board between logouts.
func TestUpdateLeaderboard_OverlaysLiveSessionMiles(t *testing.T) {
	mock := installMockDB(t)
	mock.ExpectQuery(`SELECT \* FROM "users"`).
		WithArgs(c.Conf.Platform, strings.ToLower(c.Conf.ChannelName), maxLeaderboardSize).
		WillReturnRows(leaderboardRows(
			[]driver.Value{"alice", float32(100), "twitch", false},
			[]driver.Value{"bob", float32(99), "twitch", false},
		))

	s := New(noopChatterSource{})
	// bob has been logged in for ~10 hours: 0.1mi/3min = ~20 miles in
	// progress, enough to pass alice before his logout writes it back.
	s.loggedIn["bob"] = &User{Username: "bob", Miles: 99, LoggedIn: time.Now().Add(-10 * time.Hour)}
	s.UpdateLeaderboard(context.Background())

	got := s.LifetimeLeaderboard()
	if len(got) != 2 || got[0][0] != "bob" {
		t.Fatalf("expected bob first after live-miles overlay, got %v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// A failed fetch must leave the previous board in place rather than blanking
// what the rotators display.
func TestUpdateLeaderboard_KeepsCacheOnError(t *testing.T) {
	mock := installMockDB(t)
	mock.ExpectQuery(`SELECT \* FROM "users"`).
		WillReturnError(context.DeadlineExceeded)

	s := New(noopChatterSource{})
	s.lifetimeLeaderboard = [][]string{{"alice", "100.0"}}
	s.UpdateLeaderboard(context.Background())

	if len(s.LifetimeLeaderboard()) != 1 {
		t.Fatalf("expected cache preserved on error, got %v", s.LifetimeLeaderboard())
	}
}
