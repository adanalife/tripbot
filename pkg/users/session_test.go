package users

import (
	"context"
	"database/sql/driver"
	"sync"
	"testing"
	"time"
)

// TestSessions_ConcurrentAccess hammers the session map and the leaderboard
// cache from multiple goroutines, mirroring production: the UpdateLeaderboard
// cron rebuilds the board while command dispatch reads it and the IRC-side
// calls poke the login map. Under -race this fails if Sessions loses its
// locking.
func TestSessions_ConcurrentAccess(t *testing.T) {
	const iterations = 50

	mock := installMockDB(t)
	for range iterations {
		mock.ExpectQuery(`SELECT \* FROM "users"`).
			WillReturnRows(leaderboardRows([]driver.Value{"alice", float32(100), "twitch", false}))
	}

	s := New(noopChatterSource{})
	s.loggedIn["alice"] = &User{Username: "alice", Miles: 100, LoggedIn: time.Now()}

	ctx := context.Background()
	var wg sync.WaitGroup
	for _, fn := range []func(){
		func() { s.UpdateLeaderboard(ctx) },
		func() { _ = s.LifetimeLeaderboard() },
		func() { s.GiveEveryoneMiles(0.1) },
		func() { _ = s.isLoggedIn("alice") },
		func() { _ = s.LoggedInCount() },
		func() { s.LogoutIfNecessary(ctx, "ghost") },
	} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range iterations {
				fn()
			}
		}()
	}
	wg.Wait()
}
