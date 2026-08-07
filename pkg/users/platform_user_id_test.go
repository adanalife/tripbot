package users

import (
	"context"
	"errors"
	"testing"

	"github.com/adanalife/tripbot/pkg/database/testdb"
	"gorm.io/gorm"
)

func TestFindByPlatformUserID(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	seedUsers(t, db,
		User{Username: "stamped", PlatformUserID: "12345", Miles: 10},
		User{Username: "unstamped", Miles: 20},
	)

	t.Run("finds the stamped row", func(t *testing.T) {
		u, err := FindByPlatformUserID(ctx, testConf.Platform, "12345")
		if err != nil {
			t.Fatalf("FindByPlatformUserID: %v", err)
		}
		if u.Username != "stamped" {
			t.Errorf("username = %q, want %q", u.Username, "stamped")
		}
	})

	// The empty id is the state of every row predating the column. Matching on
	// it would hand back an arbitrary unstamped user as though they were the one
	// asked for, which is worse than a miss.
	t.Run("empty id is a miss, not a wildcard", func(t *testing.T) {
		_, err := FindByPlatformUserID(ctx, testConf.Platform, "")
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("want gorm.ErrRecordNotFound for an empty id, got %v", err)
		}
	})

	t.Run("unknown id is not found", func(t *testing.T) {
		_, err := FindByPlatformUserID(ctx, testConf.Platform, "99999")
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("want gorm.ErrRecordNotFound, got %v", err)
		}
	})

	// Identity is per-platform, so an id must not cross the boundary — the same
	// numeric id on two platforms is two different people.
	t.Run("is platform scoped", func(t *testing.T) {
		_, err := FindByPlatformUserID(ctx, "youtube", "12345")
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("want gorm.ErrRecordNotFound across platforms, got %v", err)
		}
	})
}

// The uniqueness guard has to hold for stamped rows without catching the unset
// ones. Unset arrives as ” from GORM (a string field's zero value) and as NULL
// from a hand-written INSERT, and an index predicate guarding only NULL lets the
// second GORM-written row collide on ” — which is every row on this table
// until its owner next speaks.
func TestPlatformUserIDUniqueness(t *testing.T) {
	t.Run("many unstamped rows coexist", func(t *testing.T) {
		db := testdb.New(t)
		seedUsers(t, db,
			User{Username: "nobody1"},
			User{Username: "nobody2"},
			User{Username: "nobody3"},
		)
	})

	t.Run("an explicit NULL coexists with an empty string", func(t *testing.T) {
		db := testdb.New(t)
		seedUsers(t, db, User{Username: "empty"}) // GORM writes ''
		err := db.Exec(
			`INSERT INTO users (username, platform, platform_user_id) VALUES (?, ?, NULL)`,
			"null-id", testConf.Platform).Error
		if err != nil {
			t.Fatalf("a NULL id must not collide with an empty one: %v", err)
		}
	})

	t.Run("two rows cannot claim the same id", func(t *testing.T) {
		db := testdb.New(t)
		seedUsers(t, db, User{Username: "first", PlatformUserID: "12345"})
		err := db.Create(&User{
			Username: "second", Platform: testConf.Platform, PlatformUserID: "12345",
		}).Error
		if err == nil {
			t.Fatal("expected the unique index to reject a duplicate platform id")
		}
	})

	// Identity is per-platform, so the same id on two platforms is two people.
	t.Run("the same id on two platforms is allowed", func(t *testing.T) {
		db := testdb.New(t)
		seedUsers(t, db,
			User{Username: "tw", PlatformUserID: "12345", Platform: "twitch"},
			User{Username: "yt", PlatformUserID: "12345", Platform: "youtube"},
		)
	})
}

func TestRecordPlatformUserID(t *testing.T) {
	ctx := context.Background()

	t.Run("stamps a row seen for the first time", func(t *testing.T) {
		db := testdb.New(t)
		seedUsers(t, db, User{Username: "fresh"})
		u, err := Find(ctx, testConf.Platform, "fresh")
		if err != nil {
			t.Fatalf("Find: %v", err)
		}

		RecordPlatformUserID(ctx, &u, "12345")

		if u.PlatformUserID != "12345" {
			t.Errorf("in-memory id = %q, want 12345", u.PlatformUserID)
		}
		stored, err := Find(ctx, testConf.Platform, "fresh")
		if err != nil {
			t.Fatalf("Find after stamp: %v", err)
		}
		if stored.PlatformUserID != "12345" {
			t.Errorf("persisted id = %q, want 12345", stored.PlatformUserID)
		}
	})

	// The platform reporting a different id for a name means the name moved to
	// somebody else; the row follows the platform's answer.
	t.Run("re-stamps a changed id", func(t *testing.T) {
		db := testdb.New(t)
		seedUsers(t, db, User{Username: "moved", PlatformUserID: "11111"})
		u, err := Find(ctx, testConf.Platform, "moved")
		if err != nil {
			t.Fatalf("Find: %v", err)
		}

		RecordPlatformUserID(ctx, &u, "22222")

		stored, err := Find(ctx, testConf.Platform, "moved")
		if err != nil {
			t.Fatalf("Find after re-stamp: %v", err)
		}
		if stored.PlatformUserID != "22222" {
			t.Errorf("persisted id = %q, want the new 22222", stored.PlatformUserID)
		}
	})

	// A platform that reports no id must not blank an id already recorded.
	t.Run("empty id leaves a stamped row alone", func(t *testing.T) {
		db := testdb.New(t)
		seedUsers(t, db, User{Username: "keeper", PlatformUserID: "33333"})
		u, err := Find(ctx, testConf.Platform, "keeper")
		if err != nil {
			t.Fatalf("Find: %v", err)
		}

		RecordPlatformUserID(ctx, &u, "")

		stored, err := Find(ctx, testConf.Platform, "keeper")
		if err != nil {
			t.Fatalf("Find after no-op: %v", err)
		}
		if stored.PlatformUserID != "33333" {
			t.Errorf("persisted id = %q, want the untouched 33333", stored.PlatformUserID)
		}
	})

	// A transient user (no DB row) has no primary key, so an Updates() would
	// build an UPDATE with no WHERE — the same trap save() guards.
	t.Run("a row-less user is a no-op, not an error", func(t *testing.T) {
		testdb.New(t)
		u := User{Username: "transient"}
		RecordPlatformUserID(ctx, &u, "12345")
		if u.PlatformUserID != "" {
			t.Errorf("id = %q, want it left unset on a row-less user", u.PlatformUserID)
		}
	})
}
