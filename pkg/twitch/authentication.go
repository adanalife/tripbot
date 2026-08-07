package twitch

import (
	"errors"
	"log/slog"
	"time"

	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/adanalife/tripbot/pkg/oauthtokens"
)

// ErrNoToken signals "no oauth_tokens row for the bot account". Re-exported from
// oauthtokens for caller convenience.
var ErrNoToken = oauthtokens.ErrNoToken

// LoadFromDB loads both the bot row and the broadcaster row from oauth_tokens
// (which the platform-gateway keeps fresh). The bot row is required. The
// broadcaster row is optional — when missing, EventSub skips until it's seeded.
//
// Returns ErrNoToken only when the bot row is missing.
func (cl *API) LoadFromDB(botUser, broadcasterUser string) error {
	t, err := oauthtokens.Get("twitch", botUser)
	if err != nil {
		return err
	}
	cl.tokenMu.Lock()
	cl.currentUserToken = t
	cl.tokenMu.Unlock()
	instrumentation.TwitchTokenExpiry.SetExpiresAt("bot", t.ExpiresAt)

	if broadcasterUser == "" || broadcasterUser == botUser {
		return nil
	}
	bt, berr := oauthtokens.Get("twitch", broadcasterUser)
	if berr != nil {
		// Broadcaster row absent / unreadable — surface as "no token" so the
		// alert can fire instead of going silent on a missing series.
		instrumentation.TwitchTokenExpiry.SetExpiresAt("broadcaster", time.Time{})
		if errors.Is(berr, oauthtokens.ErrNoToken) {
			slog.Warn("no broadcaster oauth_tokens row; EventSub will skip until re-auth via the platform-gateway consent flow (surfaced in tripbot-console)",
				"login_as", broadcasterUser)
			return nil
		}
		slog.Error("failed to load broadcaster oauth_tokens row", "err", berr, "broadcaster", broadcasterUser)
		return nil
	}
	cl.tokenMu.Lock()
	cl.currentBroadcasterToken = bt
	cl.tokenMu.Unlock()
	instrumentation.TwitchTokenExpiry.SetExpiresAt("broadcaster", bt.ExpiresAt)
	return nil
}

// BroadcasterUserAccessToken returns the broadcaster's raw access token
// (no oauth: prefix), or "" if no broadcaster row has been loaded.
// Consumed by pkg/eventsub when subscribing to broadcaster-gated events.
func (cl *API) BroadcasterUserAccessToken() string {
	cl.tokenMu.RLock()
	defer cl.tokenMu.RUnlock()
	return cl.currentBroadcasterToken.AccessToken
}

// tokenReason classifies a loaded token: "" when usable, else "missing"
// (never loaded, or blanked by a failed refresh / invalid_grant) or
// "expired" (loaded but past ExpiresAt — a narrow window, since the gateway's
// refresh loop normally rotates ahead of expiry).
func tokenReason(t oauthtokens.Token) string {
	if t.AccessToken == "" {
		return "missing"
	}
	if !t.ExpiresAt.IsZero() && time.Now().After(t.ExpiresAt) {
		return "expired"
	}
	return ""
}

// AccountTokenStatus is the live token state for one identity, surfaced to
// tripbot-console's auth card. ExpiresAt drives an "expires in N" countdown;
// Reason is "" when healthy, else "missing"/"expired". The re-auth link itself
// lives console-side (it points at the platform-gateway consent flow), so this
// carries no URL.
type AccountTokenStatus struct {
	Account   string    // "bot" | "broadcaster"
	LoginAs   string    // the exact Twitch username
	ExpiresAt time.Time // zero when the expiry is unknown (e.g. a missing token)
	Reason    string    // "" healthy, else "missing" | "expired"
}

// TokenStatuses returns the live token state for each configured identity: the
// bot always, and the broadcaster when a distinct broadcaster identity exists
// (broadcasterUser set and != botUser). Reads in-memory token state; no DB or
// network call.
func (cl *API) TokenStatuses(botUser, broadcasterUser string) []AccountTokenStatus {
	cl.tokenMu.RLock()
	bot := cl.currentUserToken
	bcast := cl.currentBroadcasterToken
	cl.tokenMu.RUnlock()

	out := []AccountTokenStatus{{
		Account:   "bot",
		LoginAs:   botUser,
		ExpiresAt: bot.ExpiresAt,
		Reason:    tokenReason(bot),
	}}
	if broadcasterUser != "" && broadcasterUser != botUser {
		out = append(out, AccountTokenStatus{
			Account:   "broadcaster",
			LoginAs:   broadcasterUser,
			ExpiresAt: bcast.ExpiresAt,
			Reason:    tokenReason(bcast),
		})
	}
	return out
}

// Token consent + refresh are owned by the platform-gateway (gateway-twitch runs
// the OAuth consent flow and the refresh loop, and is the sole writer of
// oauth_tokens). tripbot is a token *reader*: LoadFromDB pulls the rows the
// gateway keeps fresh into the EventSub WS handshake.
