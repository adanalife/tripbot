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

func TestGet(t *testing.T) {
	db := testdb.New(t)
	tok := sampleToken()
	// Rows are written by the platform-gateway; seed one directly.
	if err := db.Create(&tok).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := Get(t.Context(), "twitch", "tripbot4000")
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
}

func TestGet_MissReturnsErrNoToken(t *testing.T) {
	testdb.New(t)
	if _, err := Get(t.Context(), "twitch", "ghost"); !errors.Is(err, ErrNoToken) {
		t.Fatalf("expected ErrNoToken, got %v", err)
	}
}
