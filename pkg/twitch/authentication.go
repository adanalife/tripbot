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
// BroadcasterScopes. See [[../decisions/...]] / vault/tripbot/tripbot/TODO.md
// "Subscriber/follower data" item for the identity-vs-scope distinction.
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

// BroadcasterClient returns the helix client that carries the broadcaster's
// user-access-token, lazy-building it on first call. Calls authorizing
// against the channel-owner identity (GetSubscriptions, GetChannelFollows
// total, channel.update) must go through this client; the bot client would
// 401 on those endpoints.
//
// The App Access Token is reused from Client(); each helix.Client only needs
// the per-identity user-access-token to differ.
func (cl *API) BroadcasterClient() (*helix.Client, error) {
	if cl.broadcasterTwitchClient != nil {
		return cl.broadcasterTwitchClient, nil
	}
	// Ensure Client() ran so appAccessToken is set.
	if _, err := cl.Client(); err != nil {
		return nil, err
	}
	client, err := helix.NewClient(&helix.Options{
		ClientID:     ClientID,
		ClientSecret: ClientSecret,
		RedirectURI:  c.Conf.ExternalURL + "/auth/callback",
		HTTPClient:   helixHTTPClient,
	})
	if err != nil {
		return nil, fmt.Errorf("twitch: building broadcaster client: %w", err)
	}
	if cl.appAccessToken != "" {
		client.SetAppAccessToken(cl.appAccessToken)
	}
	cl.tokenMu.RLock()
	if cl.currentBroadcasterToken.AccessToken != "" {
		client.SetUserAccessToken(cl.currentBroadcasterToken.AccessToken)
	}
	cl.tokenMu.RUnlock()
	cl.broadcasterTwitchClient = client
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
	if cl.broadcasterTwitchClient != nil {
		cl.broadcasterTwitchClient.SetUserAccessToken(bt.AccessToken)
	}
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
		if cl.broadcasterTwitchClient != nil {
			cl.broadcasterTwitchClient.SetUserAccessToken(tok.AccessToken)
		}
	default:
		slog.Warn("OAuth bootstrap completed for unknown identity; oauth_tokens row written but in-memory state unchanged",
			"login", u.Login, "bot", c.Conf.BotUsername, "broadcaster", c.Conf.ChannelName)
	}
	return nil
}

// RefreshUserAccessToken is the hourly cron entry point. Rotates both the
// bot's and the broadcaster's user-access-tokens if either is within 30
// minutes of expiry. Each leg uses its own Postgres advisory lock so two
// running tripbots (local dev + cluster pod) sharing the same account can't
// race the rotation.
func (cl *API) RefreshUserAccessToken(ctx context.Context) {
	botUser := c.Conf.BotUsername
	broadcasterUser := c.Conf.ChannelName

	cl.refreshOne(ctx, botUser, cl.applyBotToken, false)

	if broadcasterUser != "" && broadcasterUser != botUser {
		cl.refreshOne(ctx, broadcasterUser, cl.applyBroadcasterToken, false)
	}
}

// Reauth re-establishes a usable user-access-token for the given account
// ("bot" | "broadcaster") after an auth failure — an IRC login rejection or a
// Helix 401. It forces a refresh via the stored refresh_token (skipping the
// normal 30-minute pre-expiry window), then re-reads the row from the DB. The
// DB is the source of truth: this picks up a token just written by the
// auth-bootstrap flow (or by another tripbot's refresh) without a process
// restart. When neither yields a usable token — e.g. a DB-restored row whose
// refresh_token is also revoked — the in-memory slot is left blanked and the
// reauth link is logged; the admin panel's re-auth prompt then covers it.
func (cl *API) Reauth(ctx context.Context, account string) {
	username := c.Conf.BotUsername
	apply := cl.applyBotToken
	if account == "broadcaster" {
		username = c.Conf.ChannelName
		apply = cl.applyBroadcasterToken
	}
	cl.refreshOne(ctx, username, apply, true)
	// Re-read regardless of the refresh outcome: a concurrent auth-bootstrap
	// may have written a fresh token that a refresh with the (also-stale)
	// refresh_token couldn't produce.
	if err := cl.LoadFromDB(); err != nil {
		slog.WarnContext(ctx, "reauth: no usable token after refresh + DB re-read",
			"err", err, "login_as", username, "reauth_url", AuthInitURL(account))
	}
}

// applyBotToken writes the rotated token into the bot slot + primes the
// bot helix client. Passed to refreshOne so the per-identity slot logic
// stays out of the refresh dance itself. A zero Token (the refresh-failed
// signal) flows through naturally: the slot blanks and the gauge records 0.
func (cl *API) applyBotToken(tok oauthtokens.Token) {
	cl.tokenMu.Lock()
	cl.currentUserToken = tok
	cl.tokenMu.Unlock()
	if cl.currentTwitchClient != nil {
		cl.currentTwitchClient.SetUserAccessToken(tok.AccessToken)
	}
	instrumentation.TwitchTokenExpiry.SetExpiresAt("bot", tok.ExpiresAt)
}

// applyBroadcasterToken writes the rotated token into the broadcaster slot
// + primes the broadcaster helix client.
func (cl *API) applyBroadcasterToken(tok oauthtokens.Token) {
	cl.tokenMu.Lock()
	cl.currentBroadcasterToken = tok
	cl.tokenMu.Unlock()
	if cl.broadcasterTwitchClient != nil {
		cl.broadcasterTwitchClient.SetUserAccessToken(tok.AccessToken)
	}
	instrumentation.TwitchTokenExpiry.SetExpiresAt("broadcaster", tok.ExpiresAt)
}

// refreshOne rotates a single (provider="twitch", username) row if within
// 45 minutes of expiry. applyInMemory writes the rotated token into the
// matching in-memory slot — also called on the empty-token path so the
// caller's slot gets blanked (forcing reconnect / re-bootstrap signalling).
//
// force skips the 45-minute pre-expiry window: the hourly cron passes false
// (only rotate when close to expiry), while Reauth passes true to rotate
// immediately after an auth failure regardless of the stored ExpiresAt (which
// is unreliable after a DB restore — the dump's expiry says "valid" but Twitch
// has already invalidated the token).
//
// TODO: helix client surface should move behind an interface so this
// function can be unit-tested without a real Twitch round-trip.
func (cl *API) refreshOne(ctx context.Context, username string, applyInMemory func(oauthtokens.Token), force bool) {
	acquired, release, err := oauthtokens.TryRefreshLock("twitch", username)
	if err != nil {
		slog.ErrorContext(ctx, "oauth refresh lock acquisition failed", "err", err, "username", username)
		return
	}
	if !acquired {
		// Another process is rotating. Wait briefly, re-read the rotated
		// row, sync in-memory.
		time.Sleep(2 * time.Second)
		t, gerr := oauthtokens.Get("twitch", username)
		if gerr != nil {
			slog.ErrorContext(ctx, "post-contention re-read failed", "err", gerr, "username", username)
			return
		}
		applyInMemory(t)
		return
	}
	defer release()

	t, err := oauthtokens.Get("twitch", username)
	if err != nil {
		slog.ErrorContext(ctx, "no oauth_tokens row to refresh", "err", err, "username", username)
		return
	}

	// 45 min, not 30: Twitch's actual enforcement runs a few minutes early
	// of the published ExpiresAt — the hourly cron at 30 min was missing the
	// window and producing one self-healed 401 per 4h-ish token cycle (one
	// Sentry event each). 45 min gives the cron 15 min of headroom before
	// Twitch's variable enforcement kicks in.
	if !force && time.Until(t.ExpiresAt) > 45*time.Minute {
		return
	}

	client, err := cl.Client()
	if err != nil {
		slog.ErrorContext(ctx, "helix client unavailable for refresh", "err", err, "username", username)
		return
	}
	resp, err := client.RefreshUserAccessToken(t.RefreshToken)
	if err != nil {
		_ = oauthtokens.IncrementFailCount("twitch", username)
		slog.ErrorContext(ctx, "twitch refresh API failed", "err", err, "username", username)
		return
	}
	if resp == nil || resp.Data.AccessToken == "" {
		// Empty body typically means invalid_grant (refresh token revoked).
		// Treat as terminal — blank in-memory so callers fail loudly, and
		// Sentry surfaces the failure for re-bootstrap.
		_ = oauthtokens.IncrementFailCount("twitch", username)
		applyInMemory(oauthtokens.Token{})
		slog.ErrorContext(ctx, "oauth refresh failed; need re-bootstrap", "err", errors.New("empty access token in refresh response"), "login_as", username, "reauth_url", AuthInitURL(accountLabel(username)))
		return
	}

	rotated := oauthtokens.Token{
		Provider:     t.Provider,
		Username:     t.Username,
		TwitchUserID: t.TwitchUserID,
		AccessToken:  resp.Data.AccessToken,
		RefreshToken: resp.Data.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(resp.Data.ExpiresIn) * time.Second),
		Scopes:       strings.Join(resp.Data.Scopes, " "),
	}
	if err := oauthtokens.Upsert(rotated); err != nil {
		slog.ErrorContext(ctx, "post-refresh Upsert failed", "err", err, "username", username)
		return
	}

	applyInMemory(rotated)
	slog.InfoContext(ctx, "refreshed user access token", "username", username)
}
