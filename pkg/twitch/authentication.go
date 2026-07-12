package twitch

import (
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/adanalife/tripbot/pkg/oauthtokens"
	"github.com/nicklaw5/helix/v2"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// helixHTTPClient is the otelhttp-instrumented HTTP client passed to every
// helix.NewClient call. Without this, outbound Twitch Helix requests leave
// no trail in Tempo; with it, each helix call shows up as a span. Pairs with
// the otelhttp transports already used by pkg/vlc-client and pkg/onscreens-client.
//
// The rateLimitRecorder wraps the otelhttp transport so every Helix
// response also updates the twitch_helix_rate_limit_* gauges.
var helixHTTPClient = &http.Client{Transport: rateLimitRecorder{next: otelhttp.NewTransport(http.DefaultTransport)}}

// BotScopes is the OAuth scope set the App Access Token requests for the bot
// account (c.Conf.BotUsername — `tripbot4000` in prod). chat:read + chat:edit
// are required for IRC; moderator:read:chatters lets the bot read the viewer
// list on a channel where it is a moderator. (Interactive consent + the
// broadcaster scopes now live on the platform-gateway; these remain for the
// app-access-token request in Client.)
var BotScopes = []string{
	"chat:read",
	"chat:edit",
	"moderator:read:chatters",
}

// BroadcasterScopes is the OAuth scope set for the broadcaster account
// (c.Conf.ChannelName — `adanalife_` in prod): channel:read:subscriptions,
// moderator:read:followers, user:edit:broadcast, user:write:chat. The gateway
// owns broadcaster consent now; these remain for the app-access-token request.
var BroadcasterScopes = []string{
	"channel:read:subscriptions",
	"moderator:read:followers",
	"user:edit:broadcast",
	"user:write:chat",
}

// ErrNoToken signals "no oauth_tokens row for the bot account". Re-exported from
// oauthtokens for caller convenience.
var ErrNoToken = oauthtokens.ErrNoToken

// ClientID, ClientSecret are the static app credentials, set from env in
// init(). They are not per-instance mutable state, so they stay package-level
// (ClientID is read directly by cmd/tripbot's EventSub setup).
var (
	ClientID     string
	ClientSecret string
)

// init requires the static credentials needed to build a helix client.
// TWITCH_AUTH_TOKEN is intentionally NOT required — the IRC token lives in the
// oauth_tokens table and is loaded via LoadFromDB at boot.
func init() {
	requiredVars := []string{
		"TWITCH_CLIENT_ID",
		"TWITCH_CLIENT_SECRET",
	}
	for _, v := range requiredVars {
		if _, ok := os.LookupEnv(v); !ok {
			log.Fatalf("You must set %s", v)
		}
	}
	ClientID = os.Getenv("TWITCH_CLIENT_ID")
	ClientSecret = os.Getenv("TWITCH_CLIENT_SECRET")
}

// Client returns the shared helix client, lazy-initializing on first call.
// First-call side effect: requests an App Access Token (Client Credentials).
// The command-time Helix query surface moved to the platform-gateway; this
// client remains as the chatbot's boot-time Twitch-reachability probe.
func (cl *API) Client() (*helix.Client, error) {
	if cl.currentTwitchClient != nil {
		return cl.currentTwitchClient, nil
	}
	client, err := helix.NewClient(&helix.Options{
		ClientID:     ClientID,
		ClientSecret: ClientSecret,
		HTTPClient:   helixHTTPClient,
	})
	if err != nil {
		slog.Error("error creating twitch API client", "err", err)
		return nil, err
	}

	resp, err := client.RequestAppAccessToken(append(append([]string{}, BotScopes...), BroadcasterScopes...))
	if err != nil {
		// Twitch unreachable: RequestAppAccessToken returns a nil resp, so
		// don't touch resp.Data here (it would panic). Leave currentTwitchClient
		// uncached so the next call retries once Twitch is back — caching a
		// tokenless client would pin an empty App Access Token forever.
		slog.Error("error getting app access token from twitch", "err", err)
		return nil, err
	}
	cl.appAccessToken = resp.Data.AccessToken
	client.SetAppAccessToken(cl.appAccessToken)

	cl.currentTwitchClient = client
	return client, nil
}

// LoadFromDB loads both the bot row and the broadcaster row from oauth_tokens
// (which the platform-gateway keeps fresh). The bot row is required (no IRC
// without it). The broadcaster row is optional — when missing, EventSub skips
// until it's seeded.
//
// Returns ErrNoToken only when the bot row is missing.
func (cl *API) LoadFromDB() error {
	botUser := c.Conf.BotUsername
	broadcasterUser := c.Conf.ChannelName

	t, err := oauthtokens.Get("twitch", botUser)
	if err != nil {
		return err
	}
	cl.tokenMu.Lock()
	cl.currentUserToken = t
	cl.tokenMu.Unlock()
	if cl.currentTwitchClient != nil {
		cl.currentTwitchClient.SetUserAccessToken(t.AccessToken)
	}
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

// IRCAuthToken returns the bot's IRC oauth: token, ready for twitch.NewClient.
// Returns "" if no token has been loaded.
func (cl *API) IRCAuthToken() string {
	cl.tokenMu.RLock()
	defer cl.tokenMu.RUnlock()
	if cl.currentUserToken.AccessToken == "" {
		return ""
	}
	return "oauth:" + cl.currentUserToken.AccessToken
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
// (ChannelName set and != BotUsername). Reads in-memory token state; no DB or
// network call.
func (cl *API) TokenStatuses() []AccountTokenStatus {
	cl.tokenMu.RLock()
	bot := cl.currentUserToken
	bcast := cl.currentBroadcasterToken
	cl.tokenMu.RUnlock()

	out := []AccountTokenStatus{{
		Account:   "bot",
		LoginAs:   c.Conf.BotUsername,
		ExpiresAt: bot.ExpiresAt,
		Reason:    tokenReason(bot),
	}}
	if c.Conf.ChannelName != "" && c.Conf.ChannelName != c.Conf.BotUsername {
		out = append(out, AccountTokenStatus{
			Account:   "broadcaster",
			LoginAs:   c.Conf.ChannelName,
			ExpiresAt: bcast.ExpiresAt,
			Reason:    tokenReason(bcast),
		})
	}
	return out
}

// Token consent + refresh are owned by the platform-gateway now (gateway-twitch
// runs the OAuth consent flow and the refresh loop, and is the sole writer of
// oauth_tokens). tripbot is a token *reader*: LoadFromDB pulls the rows the
// gateway keeps fresh into the IRC PASS line + the EventSub WS handshake.
