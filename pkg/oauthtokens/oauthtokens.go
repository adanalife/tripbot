// Package oauthtokens is the storage layer for OAuth refresh + access tokens
// rotated by the bot itself. The on-disk source of truth lives in the
// `oauth_tokens` table (migration 010); this package wraps it with GORM
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
	"gorm.io/gorm"
)

// ErrNoToken is returned by Get when no row matches (provider, username).
// Callers handle this as the cold-start signal — the row is seeded by the
// platform-gateway's OAuth consent flow (gateway-<platform>/auth/init).
var ErrNoToken = errors.New("oauthtokens: no row for (provider, username); re-auth via the platform-gateway consent flow")

// Token mirrors the oauth_tokens row. TwitchUserID + LastRefreshAt are
// nullable in the schema; the bootstrap CLI populates twitch_user_id from
// helix.GetUsers, but any out-of-band INSERT (CI seed, ad-hoc psql) may
// leave it unset, so scans must handle NULL.
type Token struct {
	ID               int `gorm:"primaryKey"`
	Provider         string
	Username         string
	TwitchUserID     sql.NullString
	AccessToken      string
	RefreshToken     string
	ExpiresAt        time.Time
	Scopes           string // space-joined
	RefreshFailCount int
	LastRefreshAt    sql.NullTime
	DateCreated      time.Time
	DateUpdated      time.Time
}

// TableName overrides GORM's pluralized default ("tokens").
func (Token) TableName() string { return "oauth_tokens" }

// Get returns the row for (provider, username) or ErrNoToken if missing.
func Get(provider, username string) (Token, error) {
	var t Token
	err := database.GormDB().Where("provider = ? AND username = ?", provider, username).First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Token{}, ErrNoToken
	}
	if err != nil {
		return Token{}, fmt.Errorf("oauthtokens.Get: %w", err)
	}
	return t, nil
}

// GetByProvider returns the provider's single row, or ErrNoToken if none
// exists. Providers with one identity (YouTube: the channel owner) don't
// know a username ahead of time the way Twitch does (c.Conf.BotUsername) —
// the identity is discovered at consent time — so boot-time loading keys on
// the provider alone. If stray extra rows exist, the most recently updated
// one wins; clean strays up by hand.
func GetByProvider(provider string) (Token, error) {
	var t Token
	err := database.GormDB().Where("provider = ?", provider).Order("date_updated DESC").First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Token{}, ErrNoToken
	}
	if err != nil {
		return Token{}, fmt.Errorf("oauthtokens.GetByProvider: %w", err)
	}
	return t, nil
}

// Upsert inserts the row or updates the existing one matching (provider, username).
// On UPDATE, refresh_fail_count is reset to 0 (Upsert implies a successful refresh
// or a fresh bootstrap), last_refresh_at + date_updated are stamped to now(),
// and date_created is preserved.
func Upsert(t Token) error {
	// Raw SQL: GORM's clause.OnConflict can't express the differing
	// insert-vs-update stamping below without being harder to read.
	err := database.GormDB().Exec(`
		INSERT INTO oauth_tokens
			(provider, username, twitch_user_id, access_token, refresh_token,
			 expires_at, scopes, refresh_fail_count, last_refresh_at)
		VALUES
			(?, ?, ?, ?, ?, ?, ?, 0, NOW())
		ON CONFLICT (provider, username) DO UPDATE SET
			twitch_user_id     = EXCLUDED.twitch_user_id,
			access_token       = EXCLUDED.access_token,
			refresh_token      = EXCLUDED.refresh_token,
			expires_at         = EXCLUDED.expires_at,
			scopes             = EXCLUDED.scopes,
			refresh_fail_count = 0,
			last_refresh_at    = NOW(),
			date_updated       = NOW()
	`, t.Provider, t.Username, t.TwitchUserID, t.AccessToken, t.RefreshToken, t.ExpiresAt, t.Scopes).Error
	if err != nil {
		return fmt.Errorf("oauthtokens.Upsert: %w", err)
	}
	return nil
}

// IncrementFailCount bumps refresh_fail_count by 1 and stamps last_refresh_at.
// Called by the refresh loop on Twitch-side errors (5xx, network blip, revoked
// token) so persistent failures surface in monitoring.
func IncrementFailCount(provider, username string) error {
	res := database.GormDB().Exec(`
		UPDATE oauth_tokens
		SET refresh_fail_count = refresh_fail_count + 1,
		    last_refresh_at    = NOW(),
		    date_updated       = NOW()
		WHERE provider = ? AND username = ?
	`, provider, username)
	if res.Error != nil {
		return fmt.Errorf("oauthtokens.IncrementFailCount: %w", res.Error)
	}
	if res.RowsAffected == 0 {
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
// returns the connection to the pool. Advisory locks are session-scoped, so
// this goes through the raw *sql.DB — GORM can't pin a connection across calls.
func TryRefreshLock(provider, username string) (bool, func(), error) {
	ctx := context.Background()
	sqlDB, err := database.GormDB().DB()
	if err != nil {
		return false, nil, fmt.Errorf("oauthtokens.TryRefreshLock db: %w", err)
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return false, nil, fmt.Errorf("oauthtokens.TryRefreshLock conn: %w", err)
	}
	key := lockKey(provider, username)
	var acquired bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&acquired); err != nil {
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
