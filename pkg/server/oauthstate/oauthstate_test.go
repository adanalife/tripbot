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
		store = map[string]time.Time{}
		mu.Unlock()
		now = savedNow
	})
	mu.Lock()
	store = map[string]time.Time{}
	mu.Unlock()
}

func TestNew_GeneratesUniqueStates(t *testing.T) {
	resetStore(t)
	seen := map[string]struct{}{}
	for i := 0; i < 100; i++ {
		s := New()
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
	s := New()
	if !Validate(s) {
		t.Fatal("expected Validate(New()) to return true")
	}
}

func TestValidate_DoubleUseRejected(t *testing.T) {
	resetStore(t)
	s := New()
	if !Validate(s) {
		t.Fatal("first Validate should succeed")
	}
	if Validate(s) {
		t.Fatal("second Validate of same state should fail (single-use)")
	}
}

func TestValidate_ExpiredRejected(t *testing.T) {
	resetStore(t)
	t0 := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	now = func() time.Time { return t0 }
	s := New()
	// advance past TTL
	now = func() time.Time { return t0.Add(TTL + time.Second) }
	if Validate(s) {
		t.Fatal("expired state should not validate")
	}
}

func TestValidate_UnknownRejected(t *testing.T) {
	resetStore(t)
	if Validate("not-a-real-state") {
		t.Fatal("unknown state should not validate")
	}
}

func TestValidate_EmptyStringRejected(t *testing.T) {
	resetStore(t)
	if Validate("") {
		t.Fatal("empty state should not validate")
	}
}

func TestSweepClearsExpiredEntries(t *testing.T) {
	resetStore(t)
	t0 := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	now = func() time.Time { return t0 }
	// generate several states that will expire
	for i := 0; i < 5; i++ {
		New()
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
