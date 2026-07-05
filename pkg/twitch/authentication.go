package twitch

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/adanalife/tripbot/pkg/oauthtokens"
	"github.com/nicklaw5/helix/v2"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// helixHTTPClient is the otelhttp-instrumented HTTP client passed to every
// helix.NewClient call. Without this, outbound Twitch Helix requests leave
// no trail in Tempo; with it, each helix.GetUsers / GetSubscriptions / etc.
// shows up as a span. Pairs with the otelhttp transports already used by
// pkg/vlc-client and pkg/onscreens-client.
//
// The rateLimitRecorder wraps the otelhttp transport so every Helix
// response also updates the twitch_helix_rate_limit_* gauges — dashboards
// can see remaining headroom without waiting for a 429.
var helixHTTPClient = &http.Client{Transport: rateLimitRecorder{next: otelhttp.NewTransport(http.DefaultTransport)}}

// BotScopes is the OAuth scope set requested for the bot account
// (c.Conf.BotUsername — `tripbot4000` in prod). chat:read + chat:edit are
// required for IRC; moderator:read:chatters lets the bot read the viewer
// list on a channel where it is a moderator.
//
// Broadcaster-gated endpoints (GetSubscriptions, GetChannelFollows total)
// authorize against the broadcaster identity, not the bot — those live in
// BroadcasterScopes.
var BotScopes = []string{
	"chat:read",
	"chat:edit",
	"moderator:read:chatters",
}

// BroadcasterScopes is the OAuth scope set requested for the broadcaster
// account (c.Conf.ChannelName — `adanalife_` in prod). These are the
// Helix scopes that authorize against the channel owner's identity:
// channel:read:subscriptions for GetSubscriptions, moderator:read:followers
// for GetChannelFollows total, user:edit:broadcast for channel.update
// (title/category changes), user:write:chat for the admin console's "send as
// broadcaster" (Helix Send Chat Message posts as the authenticated user).
var BroadcasterScopes = []string{
	"channel:read:subscriptions",
	"moderator:read:followers",
	"user:edit:broadcast",
	"user:write:chat",
}

// ErrNoToken signals "no oauth_tokens row for the bot account; run the
// bootstrap CLI." Re-exported from oauthtokens for caller convenience.
var ErrNoToken = oauthtokens.ErrNoToken

// ClientID, ClientSecret are the static app credentials, set from env in
// init(). They are not per-instance mutable state, so they stay package-level
// (read directly by cmd/tripbot's EventSub setup) rather than moving onto
// Client. AppAccessToken — which IS mutable, set during Client() — lives on
// the Client struct.
var (
	ClientID     string
	ClientSecret string
)

// AuthInitURL returns the operator-facing re-bootstrap URL for the given
// account ("bot" or "broadcaster"). Visiting it kicks off the OAuth flow —
// pkg/server's /auth/init handler mints a fresh CSRF state and 302-redirects to
// Twitch — so re-auth is a click from any browser instead of running the
// bootstrap CLI from a laptop.
//
// We deliberately surface THIS URL in logs (the "token missing/expired" sites)
// rather than the fully-formed Twitch authorize URL. The latter embeds a
// single-use CSRF state with a 5-minute TTL (oauthstate.TTL) — it would be
// stale by the time anyone read the log — and that state would ship to
// Loki/Sentry. /auth/init carries no secret: client_id isn't sensitive, and no
// state exists until the redirect is generated server-side on click.
//
// We also append login_as=<username> — the exact Twitch account to sign in
// as. The /auth/init handler ignores it (it keys off account=bot|broadcaster);
// it's purely so the username is visible in the browser's address bar, matching
// the login_as attribute in the logs.
func AuthInitURL(account string) string {
	login := c.Conf.BotUsername
	if account == "broadcaster" {
		login = c.Conf.ChannelName
	}
	return c.Conf.ExternalURL + "/auth/init?account=" + account +
		"&login_as=" + url.QueryEscape(login)
}

// accountLabel maps an oauth_tokens username to the /auth/init account
// selector ("bot" or "broadcaster"). The broadcaster identity only exists
// when ChannelName differs from BotUsername; everything else is the bot.
func accountLabel(username string) string {
	if username == c.Conf.ChannelName && username != c.Conf.BotUsername {
		return "broadcaster"
	}
	return "bot"
}

// init requires the static credentials needed to build a helix client.
// TWITCH_AUTH_TOKEN is intentionally NOT required — the IRC token now lives
// in the oauth_tokens table and is loaded via LoadFromDB at boot.
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
func (cl *API) Client() (*helix.Client, error) {
	if cl.currentTwitchClient != nil {
		return cl.currentTwitchClient, nil
	}
	client, err := helix.NewClient(&helix.Options{
		ClientID:     ClientID,
		ClientSecret: ClientSecret,
		// Registered at https://dev.twitch.tv/console/apps; matched by
		// cmd/auth-bootstrap's local HTTP listener.
		RedirectURI: c.Conf.ExternalURL + "/auth/callback",
		HTTPClient:  helixHTTPClient,
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

// LoadFromDB loads both the bot row and the broadcaster row from oauth_tokens.
// The bot row is required (no IRC without it). The broadcaster row is
// optional — when missing, broadcaster-gated Helix calls skip until it's
// seeded via `task tripbot:auth:bootstrap:broadcaster`.
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
			slog.Warn("no broadcaster oauth_tokens row; subscriber/follower polling will skip until `task tripbot:auth:bootstrap:broadcaster` seeds it",
				"login_as", broadcasterUser,
				"reauth_url", AuthInitURL("broadcaster"))
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

// IRCAuthToken returns the bot's IRC oauth: token, ready for
// twitch.NewClient. Returns "" if no token has been loaded.
func (cl *API) IRCAuthToken() string {
	cl.tokenMu.RLock()
	defer cl.tokenMu.RUnlock()
	if cl.currentUserToken.AccessToken == "" {
		return ""
	}
	return "oauth:" + cl.currentUserToken.AccessToken
}

// CurrentUserAccessToken returns the raw access token (no oauth: prefix).
// Used by pkg/server/twitch.go's /auth/twitch JSON endpoint until that
// endpoint is deleted in a separate PR.
func (cl *API) CurrentUserAccessToken() string {
	cl.tokenMu.RLock()
	defer cl.tokenMu.RUnlock()
	return cl.currentUserToken.AccessToken
}

// BroadcasterUserAccessToken returns the broadcaster's raw access token
// (no oauth: prefix), or "" if no broadcaster row has been loaded.
// Consumed by pkg/eventsub when subscribing to broadcaster-gated events.
func (cl *API) BroadcasterUserAccessToken() string {
	cl.tokenMu.RLock()
	defer cl.tokenMu.RUnlock()
	return cl.currentBroadcasterToken.AccessToken
}

// AccountReauth describes an account whose in-memory token is missing or
// expired and therefore needs operator re-auth. The admin panel renders one
// "Sign in as <LoginAs>" link per entry, pointing at InitURL.
type AccountReauth struct {
	Account string // "bot" | "broadcaster" — the /auth/init account selector
	LoginAs string // the exact Twitch username to sign in as
	Reason  string // "missing" | "expired" — why re-auth is needed
	InitURL string // /auth/init URL for this account
}

// tokenReason classifies a loaded token: "" when usable, else "missing"
// (never loaded, or blanked by a failed refresh / invalid_grant) or
// "expired" (loaded but past ExpiresAt — a narrow window, since the refresh
// cron normally rotates 30m ahead).
func tokenReason(t oauthtokens.Token) string {
	if t.AccessToken == "" {
		return "missing"
	}
	if !t.ExpiresAt.IsZero() && time.Now().After(t.ExpiresAt) {
		return "expired"
	}
	return ""
}

// AccountsNeedingReauth returns the bot and/or broadcaster accounts whose
// in-memory token is missing or expired, so the admin panel can prompt for
// re-auth. Returns nil when everything's healthy. The broadcaster is only
// considered when a distinct broadcaster identity exists (ChannelName set and
// != BotUsername) — otherwise there's no separate row to re-auth.
func (cl *API) AccountsNeedingReauth() []AccountReauth {
	cl.tokenMu.RLock()
	botReason := tokenReason(cl.currentUserToken)
	bcastReason := tokenReason(cl.currentBroadcasterToken)
	cl.tokenMu.RUnlock()

	var out []AccountReauth
	if botReason != "" {
		out = append(out, AccountReauth{
			Account: "bot",
			LoginAs: c.Conf.BotUsername,
			Reason:  botReason,
			InitURL: AuthInitURL("bot"),
		})
	}
	if c.Conf.ChannelName != "" && c.Conf.ChannelName != c.Conf.BotUsername && bcastReason != "" {
		out = append(out, AccountReauth{
			Account: "broadcaster",
			LoginAs: c.Conf.ChannelName,
			Reason:  bcastReason,
			InitURL: AuthInitURL("broadcaster"),
		})
	}
	return out
}

// AccountTokenStatus is the live token state for one identity, for the admin
// panel's auth card. ExpiresAt drives a "expires in N" countdown; Reason is ""
// when healthy, else "missing"/"expired" and the panel elevates InitURL.
type AccountTokenStatus struct {
	Account   string    // "bot" | "broadcaster" — the /auth/init account selector
	LoginAs   string    // the exact Twitch username
	ExpiresAt time.Time // zero when the expiry is unknown (e.g. a missing token)
	Reason    string    // "" healthy, else "missing" | "expired"
	InitURL   string    // /auth/init URL for this account
}

// TokenStatuses returns the live token state for each configured identity: the
// bot always, and the broadcaster when a distinct broadcaster identity exists
// (ChannelName set and != BotUsername). Unlike AccountsNeedingReauth — which
// reports only the unhealthy accounts — this returns every identity so the
// panel can show a per-identity expiry countdown even while healthy. Reads
// in-memory token state; no DB or network call.
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
		InitURL:   AuthInitURL("bot"),
	}}
	if c.Conf.ChannelName != "" && c.Conf.ChannelName != c.Conf.BotUsername {
		out = append(out, AccountTokenStatus{
			Account:   "broadcaster",
			LoginAs:   c.Conf.ChannelName,
			ExpiresAt: bcast.ExpiresAt,
			Reason:    tokenReason(bcast),
			InitURL:   AuthInitURL("broadcaster"),
		})
	}
	return out
}

// ErrIdentityMismatch is returned by GenerateUserAccessToken when the
// discovered identity (via helix.GetUsers) doesn't match expectedLogin.
// Callers should surface it with retry guidance; the wrong-identity row is
// NOT written to oauth_tokens — the check runs before Upsert.
type ErrIdentityMismatch struct {
	Expected  string
	Got       string
	AccountID string // "bot" or "broadcaster" — for the retry hint
}

func (e *ErrIdentityMismatch) Error() string {
	return fmt.Sprintf("twitch: bootstrap identity mismatch — expected %q (%s) but signed in as %q. Sign out of Twitch in this browser and retry, making sure to authenticate as %q.",
		e.Expected, e.AccountID, e.Got, e.Expected)
}

// GenerateUserAccessToken exchanges a code for an access+refresh token,
// derives the authenticated username via helix.GetUsers, sanity-checks it
// against expectedLogin (when non-empty), then Upserts. When the discovered
// identity matches the bot or the broadcaster, the matching in-memory slot +
// helix client is also primed so the running pod picks up the rotated token
// without a restart.
//
// The expectedLogin check runs BEFORE Upsert so a wrong-identity sign-in
// doesn't pollute oauth_tokens. Pass "" to skip the check (legacy callers).
//
// Uses a one-shot helix client for the code exchange + GetUsers discovery
// so the shared bot client's user-access-token is never clobbered by the
// bootstrap of an unrelated identity.
//
// Returns error (no longer swallows); ErrIdentityMismatch on identity check
// failure. Callers: /auth/callback handler, cmd/auth-bootstrap.
func (cl *API) GenerateUserAccessToken(code string, expectedLogin string) error {
	// Ensure App Access Token is set so the one-shot client can fall back
	// for endpoints that allow app auth.
	if _, err := cl.Client(); err != nil {
		return err
	}
	bootstrapClient, err := helix.NewClient(&helix.Options{
		ClientID:     ClientID,
		ClientSecret: ClientSecret,
		RedirectURI:  c.Conf.ExternalURL + "/auth/callback",
		HTTPClient:   helixHTTPClient,
	})
	if err != nil {
		return fmt.Errorf("twitch: building bootstrap client: %w", err)
	}
	if cl.appAccessToken != "" {
		bootstrapClient.SetAppAccessToken(cl.appAccessToken)
	}

	resp, err := bootstrapClient.RequestUserAccessToken(code)
	if err != nil {
		return err
	}
	if resp == nil || resp.Data.AccessToken == "" {
		return errors.New("twitch: empty access token in code-exchange response")
	}

	// Set the access token so GetUsers identifies the caller from the
	// Authorization header (helix.UsersParams without IDs/Logins returns
	// the authenticated user).
	bootstrapClient.SetUserAccessToken(resp.Data.AccessToken)
	usersResp, err := bootstrapClient.GetUsers(&helix.UsersParams{})
	if err != nil {
		return err
	}
	if usersResp == nil {
		return errors.New("twitch: nil response from GetUsers")
	}
	// account="" — mid-bootstrap, re-reading the (not-yet-written) token would
	// be wrong; surface the failure to the caller instead.
	if cl.checkHelixResp(context.Background(), "GetUsers", "", &usersResp.ResponseCommon) {
		return fmt.Errorf("twitch: GetUsers returned %d during bootstrap", usersResp.StatusCode)
	}
	if len(usersResp.Data.Users) == 0 {
		return errors.New("twitch: GetUsers returned no users")
	}
	u := usersResp.Data.Users[0]

	// Sanity check: the discovered identity must match what the caller
	// initiated the flow for. Catches the "clicked the bot link with the
	// wrong browser already signed in as broadcaster" case BEFORE the row
	// gets written.
	if expectedLogin != "" && u.Login != expectedLogin {
		hint := "bot"
		if expectedLogin == c.Conf.ChannelName {
			hint = "broadcaster"
		}
		return &ErrIdentityMismatch{Expected: expectedLogin, Got: u.Login, AccountID: hint}
	}

	tok := oauthtokens.Token{
		Provider:     "twitch",
		Username:     u.Login,
		TwitchUserID: sql.NullString{String: u.ID, Valid: u.ID != ""},
		AccessToken:  resp.Data.AccessToken,
		RefreshToken: resp.Data.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(resp.Data.ExpiresIn) * time.Second),
		Scopes:       strings.Join(resp.Data.Scopes, " "),
	}
	if err := oauthtokens.Upsert(tok); err != nil {
		return err
	}

	// Route the rotated token into the right in-memory slot. Unknown identity
	// = row written but in-memory unchanged (logged so the operator notices).
	switch u.Login {
	case c.Conf.BotUsername:
		cl.tokenMu.Lock()
		cl.currentUserToken = tok
		cl.tokenMu.Unlock()
		if cl.currentTwitchClient != nil {
			cl.currentTwitchClient.SetUserAccessToken(tok.AccessToken)
		}
	case c.Conf.ChannelName:
		cl.tokenMu.Lock()
		cl.currentBroadcasterToken = tok
		cl.tokenMu.Unlock()
	default:
		slog.Warn("OAuth bootstrap completed for unknown identity; oauth_tokens row written but in-memory state unchanged",
			"login", u.Login, "bot", c.Conf.BotUsername, "broadcaster", c.Conf.ChannelName)
	}
	return nil
}

// Token refresh is owned by the platform-gateway now (gateway-twitch runs the
// refresh loop and is the sole writer of oauth_tokens). tripbot is a token
// *reader*: LoadFromDB pulls the rows the gateway keeps fresh into the IRC PASS
// line + the EventSub WS handshake. The former in-process refresh cron
// (RefreshUserAccessToken / Reauth / refreshOne) was removed so the two
// writers can't race a rotation — see the gateway-owned-Twitch-auth cutover.
