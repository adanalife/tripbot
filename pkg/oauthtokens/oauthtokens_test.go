package oauthtokens

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func newMockDB(t *testing.T) (*sqlx.DB, sqlmock.Sqlmock) {
	t.Helper()
	// Default matcher is QueryMatcherRegexp; the expected strings below are
	// substrings/patterns of the real queries.
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlx.NewDb(db, "postgres"), mock
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

func TestGet_Hit(t *testing.T) {
	db, mock := newMockDB(t)
	rows := sqlmock.NewRows([]string{
		"id", "provider", "username", "twitch_user_id", "access_token", "refresh_token",
		"expires_at", "scopes", "refresh_fail_count", "last_refresh_at",
		"date_created", "date_updated",
	}).AddRow(
		1, "twitch", "tripbot4000", "12345", "access-abc", "refresh-xyz",
		time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC),
		"chat:read chat:edit",
		0, nil,
		time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
	)
	mock.ExpectQuery(`SELECT .* FROM oauth_tokens WHERE provider=`).
		WithArgs("twitch", "tripbot4000").
		WillReturnRows(rows)

	got, err := getFromDB(db, "twitch", "tripbot4000")
	if err != nil {
		t.Fatalf("getFromDB: %v", err)
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
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT .* FROM oauth_tokens WHERE provider=`).
		WithArgs("twitch", "ghost").
		WillReturnError(sql.ErrNoRows)

	_, err := getFromDB(db, "twitch", "ghost")
	if !errors.Is(err, ErrNoToken) {
		t.Errorf("expected ErrNoToken, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestUpsert_RunsExpectedQuery(t *testing.T) {
	db, mock := newMockDB(t)
	tok := sampleToken()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO oauth_tokens`).
		WithArgs(
			tok.Provider, tok.Username, tok.TwitchUserID,
			tok.AccessToken, tok.RefreshToken, tok.ExpiresAt, tok.Scopes,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := upsertOnDB(db, tok); err != nil {
		t.Fatalf("upsertOnDB: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestUpsert_RollsBackOnExecError(t *testing.T) {
	db, mock := newMockDB(t)
	tok := sampleToken()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO oauth_tokens`).
		WillReturnError(errors.New("boom"))
	mock.ExpectRollback()

	err := upsertOnDB(db, tok)
	if err == nil {
		t.Fatal("expected error from upsertOnDB, got nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestIncrementFailCount_UpdatesRow(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec(`UPDATE oauth_tokens`).
		WithArgs("twitch", "tripbot4000").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := incrementOnDB(db, "twitch", "tripbot4000"); err != nil {
		t.Fatalf("incrementOnDB: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestIncrementFailCount_NoRowReturnsErrNoToken(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec(`UPDATE oauth_tokens`).
		WithArgs("twitch", "ghost").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := incrementOnDB(db, "twitch", "ghost")
	if !errors.Is(err, ErrNoToken) {
		t.Errorf("expected ErrNoToken, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestTryRefreshLock_Acquired(t *testing.T) {
	db, mock := newMockDB(t)
	key := lockKey("twitch", "tripbot4000")

	mock.ExpectQuery(`pg_try_advisory_lock`).
		WithArgs(key).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	mock.ExpectExec(`pg_advisory_unlock`).
		WithArgs(key).
		WillReturnResult(sqlmock.NewResult(0, 0))

	acquired, release, err := tryRefreshLockOnDB(db, "twitch", "tripbot4000")
	if err != nil {
		t.Fatalf("tryRefreshLockOnDB: %v", err)
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
	db, mock := newMockDB(t)
	key := lockKey("twitch", "tripbot4000")

	mock.ExpectQuery(`pg_try_advisory_lock`).
		WithArgs(key).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(false))

	acquired, release, err := tryRefreshLockOnDB(db, "twitch", "tripbot4000")
	if err != nil {
		t.Fatalf("tryRefreshLockOnDB: %v", err)
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
