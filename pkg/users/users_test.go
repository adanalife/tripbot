package users

import (
	"context"
	"errors"
	"testing"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database/testdb"
	"gorm.io/gorm"
)

// Find's contract: a missing user is gorm.ErrRecordNotFound, while a real DB
// failure propagates as itself — the two must never be conflated (a transient
// DB error looking like "new user" is how duplicate rows get created).
func TestFind_NotFoundVsDBError(t *testing.T) {
	t.Run("missing user surfaces gorm.ErrRecordNotFound", func(t *testing.T) {
		testdb.New(t)

		if _, err := Find(context.Background(), "ghost"); !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("want gorm.ErrRecordNotFound, got %v", err)
		}
	})

	t.Run("DB failure propagates as itself, not as not-found", func(t *testing.T) {
		db := testdb.New(t)
		seedUsers(t, db, User{Username: "somebody", Miles: 1})

		// A cancelled context is the cheapest real query failure: the row is
		// there, so a not-found return would mean the error got swallowed.
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := Find(ctx, "somebody")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want the underlying DB error, got %v", err)
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatal("DB failure must not look like not-found")
		}
	})
}

// Viewer identity is per-platform: a YouTube "ghost" is a different person from
// a Twitch "ghost", so Find must not cross the platform boundary.
func TestFind_IsPlatformScoped(t *testing.T) {
	db := testdb.New(t)
	seedUsers(t, db, User{Username: "ghost", Miles: 5, Platform: "youtube"})

	if _, err := Find(context.Background(), "ghost"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("want gorm.ErrRecordNotFound for another platform's user, got %v", err)
	}
}

// A brand-new user round-trips through the DB with a real ID, this instance's
// platform, one visit, and stamped timestamps — the autoCreateTime tags are
// what keep first_seen/date_created off the 0001-01-01 zero value (#499).
func TestFindOrCreate_CreatesRow(t *testing.T) {
	testdb.New(t)
	ctx := context.Background()

	user := FindOrCreate(ctx, "newbie")
	if user.ID == 0 {
		t.Fatal("expected a persisted user with a real ID")
	}
	if user.Username != "newbie" || user.Platform != c.Conf.Platform {
		t.Errorf("unexpected identity: %+v", user)
	}
	if user.NumVisits != 1 {
		t.Errorf("expected NumVisits=1 on create, got %d", user.NumVisits)
	}
	if user.Miles != 0 || user.IsBot || user.HasDonated {
		t.Errorf("expected zeroed defaults, got %+v", user)
	}
	for name, ts := range map[string]time.Time{
		"first_seen":   user.FirstSeen,
		"last_seen":    user.LastSeen,
		"date_created": user.DateCreated,
	} {
		if ts.Year() < 2000 {
			t.Errorf("%s not stamped on insert: %v", name, ts)
		}
	}
}

// A second FindOrCreate returns the existing row rather than inserting a
// duplicate.
func TestFindOrCreate_FindsExistingRow(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()

	first := FindOrCreate(ctx, "repeat")
	second := FindOrCreate(ctx, "repeat")
	if second.ID != first.ID {
		t.Fatalf("expected the same row, got %d then %d", first.ID, second.ID)
	}

	var count int64
	if err := db.Model(&User{}).Where("username = ?", "repeat").Count(&count).Error; err != nil {
		t.Fatalf("counting users: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 row for repeat, got %d", count)
	}
}

// save() writes the mutable columns back — the update map names them by hand,
// so a drifted column name only surfaces against a real schema.
func TestSave_PersistsMutableColumns(t *testing.T) {
	testdb.New(t)
	ctx := context.Background()

	user := FindOrCreate(ctx, "saver")
	lastSeen := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	user.Miles = 42.5
	user.NumVisits = 7
	user.IsBot = true
	user.LastSeen = lastSeen
	user.save(ctx)

	got, err := Find(ctx, "saver")
	if err != nil {
		t.Fatalf("Find after save: %v", err)
	}
	if got.Miles != 42.5 || got.NumVisits != 7 || !got.IsBot {
		t.Errorf("columns not persisted: %+v", got)
	}
	if !got.LastSeen.Equal(lastSeen) {
		t.Errorf("last_seen round-trip: want %v, got %v", lastSeen, got.LastSeen)
	}
}

// A zero ID means no DB row was ever found or created; saving would emit an
// UPDATE with no WHERE clause, so it must be refused outright.
func TestSave_ZeroIDWritesNothing(t *testing.T) {
	db := testdb.New(t)

	User{Username: "phantom", Miles: 99}.save(context.Background())

	var count int64
	if err := db.Model(&User{}).Where("username = ?", "phantom").Count(&count).Error; err != nil {
		t.Fatalf("counting users: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no row written for an ID-less user, got %d", count)
	}
}

func TestSetBot(t *testing.T) {
	t.Run("flips is_bot in the DB and in the live session", func(t *testing.T) {
		testdb.New(t)
		ctx := context.Background()

		s := New(noopChatterSource{})
		user := FindOrCreate(ctx, "maybebot")
		s.loggedIn["maybebot"] = &user

		if err := s.SetBot(ctx, "maybebot", true); err != nil {
			t.Fatalf("SetBot: %v", err)
		}

		got, err := Find(ctx, "maybebot")
		if err != nil {
			t.Fatalf("Find: %v", err)
		}
		if !got.IsBot {
			t.Error("expected is_bot persisted as true")
		}
		if live, ok := s.get("maybebot"); !ok || !live.IsBot {
			t.Errorf("expected the live session copy flipped too, got %+v", live)
		}
	})

	t.Run("unknown user returns not-found", func(t *testing.T) {
		testdb.New(t)

		s := New(noopChatterSource{})
		err := s.SetBot(context.Background(), "ghost", true)
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("want gorm.ErrRecordNotFound, got %v", err)
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
