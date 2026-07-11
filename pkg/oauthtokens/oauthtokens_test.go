package oauthtokens

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/database/testdb"
)

func sampleToken() Token {
	return Token{
		Provider:     "twitch",
		Username:     "tripbot4000",
		TwitchUserID: sql.NullString{String: "12345", Valid: true},
		AccessToken:  "access-abc",
		RefreshToken: "refresh-xyz",
		ExpiresAt:    time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC),
		Scopes:       "chat:read chat:edit channel:read:subscriptions user:edit:broadcast",
	}
}

func TestUpsertThenGet(t *testing.T) {
	testdb.New(t)
	tok := sampleToken()
	if err := Upsert(tok); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := Get("twitch", "tripbot4000")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.AccessToken != tok.AccessToken || got.RefreshToken != tok.RefreshToken || got.Scopes != tok.Scopes {
		t.Errorf("unexpected token: %+v", got)
	}
	if !got.ExpiresAt.Equal(tok.ExpiresAt) {
		t.Errorf("expires_at round-trip: want %v, got %v", tok.ExpiresAt, got.ExpiresAt)
	}
	if got.TwitchUserID.String != "12345" || !got.TwitchUserID.Valid {
		t.Errorf("unexpected twitch_user_id: %+v", got.TwitchUserID)
	}
	if got.RefreshFailCount != 0 {
		t.Errorf("expected RefreshFailCount=0, got %d", got.RefreshFailCount)
	}
	if !got.LastRefreshAt.Valid {
		t.Error("expected last_refresh_at stamped on insert")
	}
	if got.DateCreated.IsZero() || got.DateUpdated.IsZero() {
		t.Errorf("expected timestamps stamped: %+v", got)
	}
}

func TestGet_MissReturnsErrNoToken(t *testing.T) {
	testdb.New(t)
	if _, err := Get("twitch", "ghost"); !errors.Is(err, ErrNoToken) {
		t.Fatalf("expected ErrNoToken, got %v", err)
	}
}

func TestGetByProvider_MostRecentlyUpdatedWins(t *testing.T) {
	db := testdb.New(t)

	older := sampleToken()
	older.Provider, older.Username = "youtube", "UC-old"
	older.TwitchUserID = sql.NullString{}
	newer := older
	newer.Username, newer.AccessToken = "UC-new", "yt-access"
	for _, tok := range []Token{older, newer} {
		if err := Upsert(tok); err != nil {
			t.Fatalf("Upsert(%s): %v", tok.Username, err)
		}
	}
	// Upsert stamps date_updated with NOW(), which is transaction-stable —
	// both rows tie. Backdate one so the ORDER BY has something to order.
	if err := db.Exec(`UPDATE oauth_tokens SET date_updated = date_updated - interval '1 hour' WHERE username = 'UC-old'`).Error; err != nil {
		t.Fatalf("backdate: %v", err)
	}

	got, err := GetByProvider("youtube")
	if err != nil {
		t.Fatalf("GetByProvider: %v", err)
	}
	if got.Username != "UC-new" || got.AccessToken != "yt-access" {
		t.Errorf("expected most recently updated row, got %+v", got)
	}
	if got.TwitchUserID.Valid {
		t.Errorf("twitch_user_id should scan as NULL for youtube rows: %+v", got.TwitchUserID)
	}
}

func TestGetByProvider_MissReturnsErrNoToken(t *testing.T) {
	testdb.New(t)
	if _, err := GetByProvider("youtube"); !errors.Is(err, ErrNoToken) {
		t.Fatalf("expected ErrNoToken, got %v", err)
	}
}

func TestUpsert_OnConflictUpdatesInPlace(t *testing.T) {
	testdb.New(t)
	if err := Upsert(sampleToken()); err != nil {
		t.Fatalf("first Upsert: %v", err)
	}
	first, err := Get("twitch", "tripbot4000")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	// Dirty the row so the conflict path's reset is observable.
	if err := IncrementFailCount("twitch", "tripbot4000"); err != nil {
		t.Fatalf("IncrementFailCount: %v", err)
	}

	rotated := sampleToken()
	rotated.AccessToken = "access-2"
	rotated.RefreshToken = "refresh-2"
	rotated.ExpiresAt = rotated.ExpiresAt.Add(time.Hour)
	rotated.Scopes = "chat:read"
	if err := Upsert(rotated); err != nil {
		t.Fatalf("conflicting Upsert: %v", err)
	}

	got, err := Get("twitch", "tripbot4000")
	if err != nil {
		t.Fatalf("Get after upsert: %v", err)
	}
	if got.ID != first.ID {
		t.Errorf("expected in-place update of row %d, got row %d", first.ID, got.ID)
	}
	if got.AccessToken != "access-2" || got.RefreshToken != "refresh-2" || got.Scopes != "chat:read" {
		t.Errorf("row not updated: %+v", got)
	}
	if !got.ExpiresAt.Equal(rotated.ExpiresAt) {
		t.Errorf("expires_at not updated: %v", got.ExpiresAt)
	}
	if got.RefreshFailCount != 0 {
		t.Errorf("expected refresh_fail_count reset to 0, got %d", got.RefreshFailCount)
	}
	if !got.DateCreated.Equal(first.DateCreated) {
		t.Errorf("date_created not preserved: %v vs %v", got.DateCreated, first.DateCreated)
	}
}

func TestIncrementFailCount(t *testing.T) {
	testdb.New(t)
	if err := Upsert(sampleToken()); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := IncrementFailCount("twitch", "tripbot4000"); err != nil {
			t.Fatalf("IncrementFailCount: %v", err)
		}
	}
	got, err := Get("twitch", "tripbot4000")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.RefreshFailCount != 2 {
		t.Errorf("expected RefreshFailCount=2, got %d", got.RefreshFailCount)
	}
	if !got.LastRefreshAt.Valid {
		t.Error("expected last_refresh_at stamped")
	}
}

func TestIncrementFailCount_NoRowReturnsErrNoToken(t *testing.T) {
	testdb.New(t)
	if err := IncrementFailCount("twitch", "ghost"); !errors.Is(err, ErrNoToken) {
		t.Errorf("expected ErrNoToken, got %v", err)
	}
}

func TestTryRefreshLock(t *testing.T) {
	// Shared, not New: advisory locks are session-scoped, and TryRefreshLock
	// pins pooled connections a transaction-backed gorm.DB can't hand out.
	// No rows are written.
	testdb.Shared(t)

	acquired, release, err := TryRefreshLock("twitch", "tripbot4000")
	if err != nil {
		t.Fatalf("TryRefreshLock: %v", err)
	}
	if !acquired || release == nil {
		t.Fatalf("expected first acquire to succeed, got acquired=%v release-nil=%v", acquired, release == nil)
	}

	// A second session must see contention while the first holds the lock.
	contended, contendedRelease, err := TryRefreshLock("twitch", "tripbot4000")
	if err != nil {
		t.Fatalf("contended TryRefreshLock: %v", err)
	}
	if contended || contendedRelease != nil {
		t.Fatal("expected contention while lock held")
	}

	release()

	reacquired, release2, err := TryRefreshLock("twitch", "tripbot4000")
	if err != nil {
		t.Fatalf("TryRefreshLock after release: %v", err)
	}
	if !reacquired {
		t.Fatal("expected re-acquire after release")
	}
	release2()
}

func TestLockKey_StableAndDistinct(t *testing.T) {
	a := lockKey("twitch", "tripbot4000")
	b := lockKey("twitch", "tripbot4000")
	if a != b {
		t.Errorf("lockKey not stable: %d vs %d", a, b)
	}
	if lockKey("twitch", "tripbot4000") == lockKey("twitch", "adanalife") {
		t.Error("lockKey collision between distinct usernames")
	}
	if lockKey("twitch", "tripbot4000") == lockKey("github", "tripbot4000") {
		t.Error("lockKey collision between distinct providers")
	}
}
