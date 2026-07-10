package oauthtokens

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/adanalife/tripbot/pkg/database"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// installMockDB mirrors pkg/users' helper: a sqlmock-backed *gorm.DB
// installed as the process-wide singleton so package functions route to it.
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

func tokenColumns() []string {
	return []string{
		"id", "provider", "username", "twitch_user_id", "access_token", "refresh_token",
		"expires_at", "scopes", "refresh_fail_count", "last_refresh_at",
		"date_created", "date_updated",
	}
}

func TestGetByProvider_Hit(t *testing.T) {
	mock := installMockDB(t)
	rows := sqlmock.NewRows(tokenColumns()).AddRow(
		2, "youtube", "UC123", nil, "yt-access", "yt-refresh",
		time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC),
		"https://www.googleapis.com/auth/youtube.force-ssl",
		0, nil,
		time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC),
	)
	mock.ExpectQuery(`SELECT \* FROM "oauth_tokens" WHERE provider = \$1 ORDER BY date_updated DESC`).
		WithArgs("youtube", 1).
		WillReturnRows(rows)

	got, err := GetByProvider("youtube")
	if err != nil {
		t.Fatalf("GetByProvider: %v", err)
	}
	if got.Username != "UC123" || got.AccessToken != "yt-access" {
		t.Errorf("unexpected token: %+v", got)
	}
	if got.TwitchUserID.Valid {
		t.Errorf("twitch_user_id should scan as NULL for youtube rows: %+v", got.TwitchUserID)
	}
}

func TestGetByProvider_MissReturnsErrNoToken(t *testing.T) {
	mock := installMockDB(t)
	mock.ExpectQuery(`SELECT \* FROM "oauth_tokens" WHERE provider = \$1 ORDER BY date_updated DESC`).
		WithArgs("youtube", 1).
		WillReturnRows(sqlmock.NewRows(tokenColumns()))

	if _, err := GetByProvider("youtube"); !errors.Is(err, ErrNoToken) {
		t.Fatalf("expected ErrNoToken, got %v", err)
	}
}

func TestGet_Hit(t *testing.T) {
	mock := installMockDB(t)
	rows := sqlmock.NewRows(tokenColumns()).AddRow(
		1, "twitch", "tripbot4000", "12345", "access-abc", "refresh-xyz",
		time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC),
		"chat:read chat:edit",
		0, nil,
		time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
	)
	mock.ExpectQuery(`SELECT \* FROM "oauth_tokens" WHERE provider = \$1 AND username = \$2`).
		WithArgs("twitch", "tripbot4000", 1).
		WillReturnRows(rows)

	got, err := Get("twitch", "tripbot4000")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Username != "tripbot4000" || got.AccessToken != "access-abc" || got.RefreshToken != "refresh-xyz" {
		t.Errorf("unexpected token: %+v", got)
	}
	if got.RefreshFailCount != 0 {
		t.Errorf("expected RefreshFailCount=0, got %d", got.RefreshFailCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestGet_Miss(t *testing.T) {
	mock := installMockDB(t)
	mock.ExpectQuery(`SELECT \* FROM "oauth_tokens" WHERE provider = \$1 AND username = \$2`).
		WithArgs("twitch", "ghost", 1).
		WillReturnRows(sqlmock.NewRows(tokenColumns()))

	_, err := Get("twitch", "ghost")
	if !errors.Is(err, ErrNoToken) {
		t.Errorf("expected ErrNoToken, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestUpsert_RunsExpectedQuery(t *testing.T) {
	mock := installMockDB(t)
	tok := sampleToken()

	mock.ExpectExec(`INSERT INTO oauth_tokens`).
		WithArgs(
			tok.Provider, tok.Username, tok.TwitchUserID,
			tok.AccessToken, tok.RefreshToken, tok.ExpiresAt, tok.Scopes,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := Upsert(tok); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestUpsert_PropagatesExecError(t *testing.T) {
	mock := installMockDB(t)
	mock.ExpectExec(`INSERT INTO oauth_tokens`).
		WillReturnError(errors.New("boom"))

	if err := Upsert(sampleToken()); err == nil {
		t.Fatal("expected error from Upsert, got nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestIncrementFailCount_UpdatesRow(t *testing.T) {
	mock := installMockDB(t)
	mock.ExpectExec(`UPDATE oauth_tokens`).
		WithArgs("twitch", "tripbot4000").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := IncrementFailCount("twitch", "tripbot4000"); err != nil {
		t.Fatalf("IncrementFailCount: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestIncrementFailCount_NoRowReturnsErrNoToken(t *testing.T) {
	mock := installMockDB(t)
	mock.ExpectExec(`UPDATE oauth_tokens`).
		WithArgs("twitch", "ghost").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := IncrementFailCount("twitch", "ghost")
	if !errors.Is(err, ErrNoToken) {
		t.Errorf("expected ErrNoToken, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestTryRefreshLock_Acquired(t *testing.T) {
	mock := installMockDB(t)
	key := lockKey("twitch", "tripbot4000")

	mock.ExpectQuery(`pg_try_advisory_lock`).
		WithArgs(key).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	mock.ExpectExec(`pg_advisory_unlock`).
		WithArgs(key).
		WillReturnResult(sqlmock.NewResult(0, 0))

	acquired, release, err := TryRefreshLock("twitch", "tripbot4000")
	if err != nil {
		t.Fatalf("TryRefreshLock: %v", err)
	}
	if !acquired {
		t.Fatal("expected acquired=true")
	}
	if release == nil {
		t.Fatal("expected non-nil release fn")
	}
	release()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestTryRefreshLock_Contended(t *testing.T) {
	mock := installMockDB(t)
	key := lockKey("twitch", "tripbot4000")

	mock.ExpectQuery(`pg_try_advisory_lock`).
		WithArgs(key).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(false))

	acquired, release, err := TryRefreshLock("twitch", "tripbot4000")
	if err != nil {
		t.Fatalf("TryRefreshLock: %v", err)
	}
	if acquired {
		t.Fatal("expected acquired=false")
	}
	if release != nil {
		t.Fatal("expected nil release fn on contention")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
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
