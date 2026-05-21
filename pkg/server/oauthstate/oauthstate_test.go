package oauthstate

import (
	"testing"
	"time"
)

func resetStore(t *testing.T) {
	t.Helper()
	savedNow := now
	t.Cleanup(func() {
		mu.Lock()
		store = map[string]entry{}
		mu.Unlock()
		now = savedNow
	})
	mu.Lock()
	store = map[string]entry{}
	mu.Unlock()
}

func TestNew_GeneratesUniqueStates(t *testing.T) {
	resetStore(t)
	seen := map[string]struct{}{}
	for i := 0; i < 100; i++ {
		s := New(AccountUnchecked)
		if s == "" {
			t.Fatal("New returned empty state")
		}
		if _, dup := seen[s]; dup {
			t.Fatalf("duplicate state generated: %q", s)
		}
		seen[s] = struct{}{}
	}
}

func TestValidate_Hit(t *testing.T) {
	resetStore(t)
	s := New(AccountUnchecked)
	if _, ok := Validate(s); !ok {
		t.Fatal("expected Validate(New()) to return true")
	}
}

func TestValidate_RoundTripsAccount(t *testing.T) {
	resetStore(t)
	s := New(AccountBroadcaster)
	got, ok := Validate(s)
	if !ok {
		t.Fatal("expected Validate to succeed")
	}
	if got != AccountBroadcaster {
		t.Errorf("Validate returned account %q, want %q", got, AccountBroadcaster)
	}
}

func TestValidate_DoubleUseRejected(t *testing.T) {
	resetStore(t)
	s := New(AccountBot)
	if _, ok := Validate(s); !ok {
		t.Fatal("first Validate should succeed")
	}
	if _, ok := Validate(s); ok {
		t.Fatal("second Validate of same state should fail (single-use)")
	}
}

func TestValidate_ExpiredRejected(t *testing.T) {
	resetStore(t)
	t0 := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	now = func() time.Time { return t0 }
	s := New(AccountBot)
	// advance past TTL
	now = func() time.Time { return t0.Add(TTL + time.Second) }
	if _, ok := Validate(s); ok {
		t.Fatal("expired state should not validate")
	}
}

func TestValidate_UnknownRejected(t *testing.T) {
	resetStore(t)
	if _, ok := Validate("not-a-real-state"); ok {
		t.Fatal("unknown state should not validate")
	}
}

func TestValidate_EmptyStringRejected(t *testing.T) {
	resetStore(t)
	if _, ok := Validate(""); ok {
		t.Fatal("empty state should not validate")
	}
}

func TestSweepClearsExpiredEntries(t *testing.T) {
	resetStore(t)
	t0 := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	now = func() time.Time { return t0 }
	// generate several states that will expire
	for i := 0; i < 5; i++ {
		New(AccountUnchecked)
	}
	mu.Lock()
	beforeSweep := len(store)
	mu.Unlock()
	if beforeSweep != 5 {
		t.Fatalf("expected 5 entries before sweep, got %d", beforeSweep)
	}
	// jump past TTL, then Validate (which triggers sweep)
	now = func() time.Time { return t0.Add(TTL + time.Second) }
	Validate("anything") // miss, but triggers sweep
	mu.Lock()
	afterSweep := len(store)
	mu.Unlock()
	if afterSweep != 0 {
		t.Fatalf("expected sweep to clear expired entries, %d remain", afterSweep)
	}
}
