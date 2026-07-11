package users

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/database/testdb"
	"github.com/adanalife/tripbot/pkg/events"
)

// TestSessions_ConcurrentAccess hammers the session map and the leaderboard
// cache from multiple goroutines, mirroring production: the UpdateLeaderboard
// cron rebuilds the board while command dispatch reads it and the IRC-side
// calls poke the login map. Under -race this fails if Sessions loses its
// locking.
func TestSessions_ConcurrentAccess(t *testing.T) {
	const iterations = 50

	db := testdb.New(t)
	seedUsers(t, db, User{Username: "alice", Miles: 100})

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

// A login creates the DB row, counts the visit, and records a login event
// carrying the session ID that the eventual logout pairs with.
func TestLoginIfNecessary_PersistsVisitAndEvent(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()

	s := New(noopChatterSource{})
	user := s.LoginIfNecessary(ctx, "arrival")
	if user.ID == 0 {
		t.Fatal("expected a persisted user")
	}

	stored, err := Find(ctx, "arrival")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	// create() opens with one visit; login() counts this one on top.
	if stored.NumVisits != 2 {
		t.Errorf("expected NumVisits=2 after login, got %d", stored.NumVisits)
	}

	var loginEvents []events.Event
	if err := db.Where("username = ? AND event = ?", "arrival", "login").Find(&loginEvents).Error; err != nil {
		t.Fatalf("reading events: %v", err)
	}
	if len(loginEvents) != 1 {
		t.Fatalf("expected 1 login event, got %d", len(loginEvents))
	}
	if loginEvents[0].SessionID != user.sessionID {
		t.Errorf("login event session_id %v does not match the session's %v",
			loginEvents[0].SessionID, user.sessionID)
	}

	// An already-logged-in user is not logged in twice.
	if again := s.LoginIfNecessary(ctx, "arrival"); again.sessionID != user.sessionID {
		t.Error("expected the existing session to be reused")
	}
	if s.LoggedInCount() != 1 {
		t.Errorf("expected 1 logged-in user, got %d", s.LoggedInCount())
	}
}

// Logging out banks the session's miles to the users row, drops the user from
// the session, and closes the pairing with a logout event.
func TestLogoutIfNecessary_BanksMilesAndClosesSession(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()

	s := New(noopChatterSource{})
	user := s.LoginIfNecessary(ctx, "departure")
	// Backdate the login so the session banks a non-zero mileage:
	// 0.1mi/3min means ~20 miles for ten hours in chat.
	s.mu.Lock()
	s.loggedIn["departure"].LoggedIn = time.Now().Add(-10 * time.Hour)
	s.mu.Unlock()

	s.LogoutIfNecessary(ctx, "departure")

	if s.LoggedInCount() != 0 {
		t.Errorf("expected an empty session after logout, got %d", s.LoggedInCount())
	}
	stored, err := Find(ctx, "departure")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if stored.Miles < 15 || stored.Miles > 25 {
		t.Errorf("expected ~20 session miles banked, got %v", stored.Miles)
	}

	var logoutEvents []events.Event
	if err := db.Where("username = ? AND event = ?", "departure", "logout").Find(&logoutEvents).Error; err != nil {
		t.Fatalf("reading events: %v", err)
	}
	if len(logoutEvents) != 1 {
		t.Fatalf("expected 1 logout event, got %d", len(logoutEvents))
	}
	if logoutEvents[0].SessionID != user.sessionID {
		t.Errorf("logout event does not pair with the login session ID")
	}
	// No sub-grants and not a subscriber, so the bonus column stays NULL.
	if logoutEvents[0].ExtraMilesEarned != nil {
		t.Errorf("expected extra_miles_earned NULL, got %v", *logoutEvents[0].ExtraMilesEarned)
	}
}

// Community sub-grants are unreconstructable from the login/logout pairing, so
// logout records them on the event as extra_miles_earned.
func TestLogout_RecordsGiftedMilesAsExtra(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()

	s := New(noopChatterSource{})
	s.LoginIfNecessary(ctx, "gifted")
	s.GiveEveryoneMiles(5)
	s.LogoutIfNecessary(ctx, "gifted")

	var logoutEvents []events.Event
	if err := db.Where("username = ? AND event = ?", "gifted", "logout").Find(&logoutEvents).Error; err != nil {
		t.Fatalf("reading events: %v", err)
	}
	if len(logoutEvents) != 1 {
		t.Fatalf("expected 1 logout event, got %d", len(logoutEvents))
	}
	if logoutEvents[0].ExtraMilesEarned == nil || *logoutEvents[0].ExtraMilesEarned != 5 {
		t.Errorf("expected extra_miles_earned=5, got %v", logoutEvents[0].ExtraMilesEarned)
	}
}

// CorrectMiles applies a manual delta and persists it, whether or not the user
// is currently in chat.
func TestCorrectMiles(t *testing.T) {
	t.Run("logged-out user is corrected in the DB", func(t *testing.T) {
		db := testdb.New(t)
		ctx := context.Background()
		seedUsers(t, db, User{Username: "offline", Miles: 10})

		s := New(noopChatterSource{})
		if got := s.CorrectMiles(ctx, "offline", -4); got != 6 {
			t.Errorf("expected 6 miles returned, got %v", got)
		}

		stored, err := Find(ctx, "offline")
		if err != nil {
			t.Fatalf("Find: %v", err)
		}
		if stored.Miles != 6 {
			t.Errorf("expected 6 miles persisted, got %v", stored.Miles)
		}
	})

	t.Run("logged-in user's live copy is corrected too", func(t *testing.T) {
		testdb.New(t)
		ctx := context.Background()

		s := New(noopChatterSource{})
		s.LoginIfNecessary(ctx, "online")
		s.CorrectMiles(ctx, "online", 12)

		live, ok := s.get("online")
		if !ok {
			t.Fatal("expected the user to still be logged in")
		}
		if live.Miles != 12 {
			t.Errorf("expected the live copy corrected, got %v", live.Miles)
		}
		stored, err := Find(ctx, "online")
		if err != nil {
			t.Fatalf("Find: %v", err)
		}
		if stored.Miles != 12 {
			t.Errorf("expected 12 miles persisted, got %v", stored.Miles)
		}
	})
}
