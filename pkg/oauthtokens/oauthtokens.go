// Package oauthtokens is the storage layer for OAuth refresh + access tokens
// rotated by the bot itself. The on-disk source of truth lives in the
// `oauth_tokens` table (migration 010); this package wraps it with sqlx queries
// matching the rest of tripbot's DB access pattern.
package oauthtokens

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/adanalife/tripbot/pkg/database"
	"github.com/jmoiron/sqlx"
)

// ErrNoToken is returned by Get when no row matches (provider, username).
// Callers handle this as the cold-start signal — typically log.Fatal pointing
// at the bootstrap CLI.
var ErrNoToken = errors.New("oauthtokens: no row for (provider, username); run task tripbot:auth:bootstrap")

// Token mirrors the oauth_tokens row.
type Token struct {
	ID               int          `db:"id"`
	Provider         string       `db:"provider"`
	Username         string       `db:"username"`
	TwitchUserID     string       `db:"twitch_user_id"`
	AccessToken      string       `db:"access_token"`
	RefreshToken     string       `db:"refresh_token"`
	ExpiresAt        time.Time    `db:"expires_at"`
	Scopes           string       `db:"scopes"` // space-joined
	RefreshFailCount int          `db:"refresh_fail_count"`
	LastRefreshAt    sql.NullTime `db:"last_refresh_at"`
	DateCreated      time.Time    `db:"date_created"`
	DateUpdated      time.Time    `db:"date_updated"`
}

// Get returns the row for (provider, username) or ErrNoToken if missing.
func Get(provider, username string) (Token, error) {
	return getFromDB(database.Connection(), provider, username)
}

func getFromDB(db *sqlx.DB, provider, username string) (Token, error) {
	var t Token
	query := `SELECT * FROM oauth_tokens WHERE provider=$1 AND username=$2`
	err := db.Get(&t, query, provider, username)
	if errors.Is(err, sql.ErrNoRows) {
		return Token{}, ErrNoToken
	}
	if err != nil {
		return Token{}, fmt.Errorf("oauthtokens.Get: %w", err)
	}
	return t, nil
}

// Upsert inserts the row or updates the existing one matching (provider, username).
// On UPDATE, refresh_fail_count is reset to 0 (Upsert implies a successful refresh
// or a fresh bootstrap), last_refresh_at + date_updated are stamped to now(),
// and date_created is preserved.
func Upsert(t Token) error {
	return upsertOnDB(database.Connection(), t)
}

func upsertOnDB(db *sqlx.DB, t Token) error {
	tx, err := db.Beginx()
	if err != nil {
		return fmt.Errorf("oauthtokens.Upsert begin: %w", err)
	}
	query := `
		INSERT INTO oauth_tokens
			(provider, username, twitch_user_id, access_token, refresh_token,
			 expires_at, scopes, refresh_fail_count, last_refresh_at)
		VALUES
			(:provider, :username, :twitch_user_id, :access_token, :refresh_token,
			 :expires_at, :scopes, 0, NOW())
		ON CONFLICT (provider, username) DO UPDATE SET
			twitch_user_id     = EXCLUDED.twitch_user_id,
			access_token       = EXCLUDED.access_token,
			refresh_token      = EXCLUDED.refresh_token,
			expires_at         = EXCLUDED.expires_at,
			scopes             = EXCLUDED.scopes,
			refresh_fail_count = 0,
			last_refresh_at    = NOW(),
			date_updated       = NOW()
	`
	if _, err := tx.NamedExec(query, t); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("oauthtokens.Upsert exec: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("oauthtokens.Upsert commit: %w", err)
	}
	return nil
}

// IncrementFailCount bumps refresh_fail_count by 1 and stamps last_refresh_at.
// Called by the refresh loop on Twitch-side errors (5xx, network blip, revoked
// token) so persistent failures surface in monitoring.
func IncrementFailCount(provider, username string) error {
	return incrementOnDB(database.Connection(), provider, username)
}

func incrementOnDB(db *sqlx.DB, provider, username string) error {
	query := `
		UPDATE oauth_tokens
		SET refresh_fail_count = refresh_fail_count + 1,
		    last_refresh_at    = NOW(),
		    date_updated       = NOW()
		WHERE provider=$1 AND username=$2
	`
	res, err := db.Exec(query, provider, username)
	if err != nil {
		return fmt.Errorf("oauthtokens.IncrementFailCount: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("oauthtokens.IncrementFailCount rows: %w", err)
	}
	if n == 0 {
		return ErrNoToken
	}
	return nil
}

// TryRefreshLock attempts to acquire a Postgres session-scoped advisory lock
// keyed on (provider, username). The lock prevents concurrent refresh between
// a local-dev tripbot and a cluster pod sharing the same Twitch account: only
// one can rotate the refresh token at a time.
//
// On success, acquired=true and the caller MUST invoke release() when done.
// On contention, acquired=false; the caller should re-Get the row (the winner
// has rotated it) and proceed without calling release().
//
// The lock is held against a single pooled connection; release() unlocks and
// returns the connection to the pool.
func TryRefreshLock(provider, username string) (bool, func(), error) {
	return tryRefreshLockOnDB(database.Connection(), provider, username)
}

func tryRefreshLockOnDB(db *sqlx.DB, provider, username string) (bool, func(), error) {
	ctx := context.Background()
	conn, err := db.Connx(ctx)
	if err != nil {
		return false, nil, fmt.Errorf("oauthtokens.TryRefreshLock conn: %w", err)
	}
	key := lockKey(provider, username)
	var acquired bool
	if err := conn.GetContext(ctx, &acquired, "SELECT pg_try_advisory_lock($1)", key); err != nil {
		_ = conn.Close()
		return false, nil, fmt.Errorf("oauthtokens.TryRefreshLock acquire: %w", err)
	}
	if !acquired {
		_ = conn.Close()
		return false, nil, nil
	}
	release := func() {
		// Best-effort unlock; closing the conn releases it regardless.
		_, _ = conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", key)
		_ = conn.Close()
	}
	return acquired, release, nil
}

// lockKey hashes the lock identifier into a stable int64 suitable for
// pg_try_advisory_lock(bigint). The hashtext() approach used elsewhere is
// 32-bit; we use SHA-256 → first 8 bytes as int64 for a wider key space.
func lockKey(provider, username string) int64 {
	h := sha256.Sum256([]byte("oauth_refresh:" + provider + ":" + username))
	return int64(binary.BigEndian.Uint64(h[:8]))
}
