package users

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"gorm.io/gorm"
)

// Find's contract: a missing user is gorm.ErrRecordNotFound, while a real DB
// failure propagates as itself — the two must never be conflated (a transient
// DB error looking like "new user" is how duplicate rows get created).
func TestFind_NotFoundVsDBError(t *testing.T) {
	t.Run("missing user surfaces gorm.ErrRecordNotFound", func(t *testing.T) {
		mock := installMockDB(t)
		mock.ExpectQuery(`SELECT \* FROM "users"`).
			WithArgs(c.Conf.Platform, "ghost", 1).
			WillReturnRows(sqlmock.NewRows([]string{"id"}))

		_, err := Find(context.Background(), "ghost")
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("want gorm.ErrRecordNotFound, got %v", err)
		}
	})

	t.Run("DB failure propagates as itself, not as not-found", func(t *testing.T) {
		mock := installMockDB(t)
		dbErr := errors.New("connection refused")
		mock.ExpectQuery(`SELECT \* FROM "users"`).WillReturnError(dbErr)

		_, err := Find(context.Background(), "somebody")
		if !errors.Is(err, dbErr) {
			t.Fatalf("want underlying DB error, got %v", err)
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatal("DB failure must not look like not-found")
		}
	})
}

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
		if !u.HasGuessCommandAvailable(context.Background(), time.Time{}) {
			t.Fatal("expected true for fresh user")
		}
	})

	t.Run("recent guess on cooldown returns false", func(t *testing.T) {
		u := User{lastLocation: time.Now()}
		if u.HasGuessCommandAvailable(context.Background(), time.Time{}) {
			t.Fatal("expected false during cooldown")
		}
	})

	t.Run("expired cooldown returns true", func(t *testing.T) {
		u := User{lastLocation: time.Now().Add(-2 * guessCooldown)}
		if !u.HasGuessCommandAvailable(context.Background(), time.Time{}) {
			t.Fatal("expected true after cooldown expires")
		}
	})

	t.Run("timewarp after lastLocation overrides cooldown", func(t *testing.T) {
		now := time.Now()
		u := User{lastLocation: now.Add(-30 * time.Second)}
		recentTimewarp := now.Add(-10 * time.Second)
		if !u.HasGuessCommandAvailable(context.Background(), recentTimewarp) {
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
