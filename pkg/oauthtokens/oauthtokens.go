// Package oauthtokens reads the OAuth refresh + access tokens stored in the
// `oauth_tokens` table (migration 010). The platform-gateway owns writes and
// refreshes; this package wraps the read with GORM matching the rest of
// tripbot's DB access pattern.
package oauthtokens

import (
	"context"
	"database/sql"
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
func Get(ctx context.Context, provider, username string) (Token, error) {
	var t Token
	err := database.GormDB().WithContext(ctx).Where("provider = ? AND username = ?", provider, username).First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Token{}, ErrNoToken
	}
	if err != nil {
		return Token{}, fmt.Errorf("oauthtokens.Get: %w", err)
	}
	return t, nil
}
