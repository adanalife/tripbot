package users

import (
	"testing"
	"time"
)

func TestGuessCooldownRemaining(t *testing.T) {
	t.Run("zero lastLocation returns no cooldown", func(t *testing.T) {
		u := User{}
		if got := u.GuessCooldownRemaining(); got != 0 {
			t.Fatalf("got %v, want 0", got)
		}
	})

	t.Run("recent lastLocation returns positive cooldown", func(t *testing.T) {
		u := User{lastLocation: time.Now().Add(-1 * time.Minute)}
		got := u.GuessCooldownRemaining()
		if got <= 0 {
			t.Fatalf("expected positive cooldown, got %v", got)
		}
		if got > guessCooldown {
			t.Fatalf("cooldown %v exceeds max %v", got, guessCooldown)
		}
	})

	t.Run("expired cooldown returns zero", func(t *testing.T) {
		u := User{lastLocation: time.Now().Add(-2 * guessCooldown)}
		if got := u.GuessCooldownRemaining(); got != 0 {
			t.Fatalf("got %v, want 0", got)
		}
	})
}

func TestHasGuessCommandAvailable(t *testing.T) {
	t.Run("never guessed returns true", func(t *testing.T) {
		u := User{}
		if !u.HasGuessCommandAvailable(time.Time{}) {
			t.Fatal("expected true for fresh user")
		}
	})

	t.Run("recent guess on cooldown returns false", func(t *testing.T) {
		u := User{lastLocation: time.Now()}
		if u.HasGuessCommandAvailable(time.Time{}) {
			t.Fatal("expected false during cooldown")
		}
	})

	t.Run("expired cooldown returns true", func(t *testing.T) {
		u := User{lastLocation: time.Now().Add(-2 * guessCooldown)}
		if !u.HasGuessCommandAvailable(time.Time{}) {
			t.Fatal("expected true after cooldown expires")
		}
	})

	t.Run("timewarp after lastLocation overrides cooldown", func(t *testing.T) {
		now := time.Now()
		u := User{lastLocation: now.Add(-30 * time.Second)}
		recentTimewarp := now.Add(-10 * time.Second)
		if !u.HasGuessCommandAvailable(recentTimewarp) {
			t.Fatal("expected true when timewarp is more recent than lastLocation")
		}
	})
}

func TestSetLastLocationTime(t *testing.T) {
	u := &User{}
	before := time.Now()
	u.SetLastLocationTime()
	after := time.Now()
	if u.lastLocation.Before(before) || u.lastLocation.After(after) {
		t.Fatalf("lastLocation %v not in [%v, %v]", u.lastLocation, before, after)
	}
}
