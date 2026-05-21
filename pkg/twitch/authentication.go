package twitch

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
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
// (title/category changes).
var BroadcasterScopes = []string{
	"channel:read:subscriptions",
	"moderator:read:followers",
	"user:edit:broadcast",
}

// ErrNoToken signals "no oauth_tokens row for the bot account; run the
// bootstrap CLI." Re-exported from oauthtokens for caller convenience.
var ErrNoToken = oauthtokens.ErrNoToken

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
func AuthInitURL(account string) string {
	return c.Conf.ExternalURL + "/auth/init?account=" + account
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

// currentTwitchClient is the lazy-initialized bot helix client. IRC auth +
// any Helix endpoint authorized against the bot's identity goes through this.
var currentTwitchClient *helix.Client

// broadcasterTwitchClient is the lazy-initialized broadcaster helix client.
// Endpoints that authorize against the channel-owner identity (GetSubscriptions,
// GetChannelFollows total, channel.update, mod actions) go through this — the
// bot client would 401 since the user-access-token's identity matters, not
// just its scope set.
var broadcasterTwitchClient *helix.Client

// ClientID, ClientSecret are set from env at init.
// AppAccessToken is set in Client() (Client Credentials grant) and shared by
// both helix clients.
var (
	ClientID       string
	ClientSecret   string
	AppAccessToken string
)

// tokenMu guards both currentUserToken (bot) and currentBroadcasterToken.
// RWMutex because reads (IRCAuthToken, CurrentUserAccessToken) outnumber
// writes (LoadFromDB, refresh).
var (
	tokenMu                 sync.RWMutex
	currentUserToken        oauthtokens.Token
	currentBroadcasterToken oauthtokens.Token
)

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
func Client() (*helix.Client, error) {
	if currentTwitchClient != nil {
		return currentTwitchClient, nil
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
	AppAccessToken = resp.Data.AccessToken
	client.SetAppAccessToken(AppAccessToken)

	currentTwitchClient = client
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
func BroadcasterClient() (*helix.Client, error) {
	if broadcasterTwitchClient != nil {
		return broadcasterTwitchClient, nil
	}
	// Ensure Client() ran so AppAccessToken is set.
	if _, err := Client(); err != nil {
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
	if AppAccessToken != "" {
		client.SetAppAccessToken(AppAccessToken)
	}
	tokenMu.RLock()
	if currentBroadcasterToken.AccessToken != "" {
		client.SetUserAccessToken(currentBroadcasterToken.AccessToken)
	}
	tokenMu.RUnlock()
	broadcasterTwitchClient = client
	return client, nil
}

// LoadFromDB loads both the bot row and the broadcaster row from oauth_tokens.
// The bot row is required (no IRC without it). The broadcaster row is
// optional — when missing, broadcaster-gated Helix calls skip until it's
// seeded via `task tripbot:auth:bootstrap:broadcaster`.
//
// Returns ErrNoToken only when the bot row is missing.
func LoadFromDB() error {
	botUser := c.Conf.BotUsername
	broadcasterUser := c.Conf.ChannelName

	t, err := oauthtokens.Get("twitch", botUser)
	if err != nil {
		return err
	}
	tokenMu.Lock()
	currentUserToken = t
	tokenMu.Unlock()
	if currentTwitchClient != nil {
		currentTwitchClient.SetUserAccessToken(t.AccessToken)
	}

	if broadcasterUser == "" || broadcasterUser == botUser {
		return nil
	}
	bt, berr := oauthtokens.Get("twitch", broadcasterUser)
	if berr != nil {
		if errors.Is(berr, oauthtokens.ErrNoToken) {
			slog.Warn("no broadcaster oauth_tokens row; subscriber/follower polling will skip until `task tripbot:auth:bootstrap:broadcaster` seeds it",
				"broadcaster", broadcasterUser)
			return nil
		}
		slog.Error("failed to load broadcaster oauth_tokens row", "err", berr, "broadcaster", broadcasterUser)
		return nil
	}
	tokenMu.Lock()
	currentBroadcasterToken = bt
	tokenMu.Unlock()
	if broadcasterTwitchClient != nil {
		broadcasterTwitchClient.SetUserAccessToken(bt.AccessToken)
	}
	return nil
}

// IRCAuthToken returns the bot's IRC oauth: token, ready for
// twitch.NewClient. Returns "" if no token has been loaded.
func IRCAuthToken() string {
	tokenMu.RLock()
	defer tokenMu.RUnlock()
	if currentUserToken.AccessToken == "" {
		return ""
	}
	return "oauth:" + currentUserToken.AccessToken
}

// CurrentUserAccessToken returns the raw access token (no oauth: prefix).
// Used by pkg/server/twitch.go's /auth/twitch JSON endpoint until that
// endpoint is deleted in a separate PR.
func CurrentUserAccessToken() string {
	tokenMu.RLock()
	defer tokenMu.RUnlock()
	return currentUserToken.AccessToken
}

// BroadcasterUserAccessToken returns the broadcaster's raw access token
// (no oauth: prefix), or "" if no broadcaster row has been loaded.
// Consumed by pkg/eventsub when subscribing to broadcaster-gated events.
func BroadcasterUserAccessToken() string {
	tokenMu.RLock()
	defer tokenMu.RUnlock()
	return currentBroadcasterToken.AccessToken
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
func GenerateUserAccessToken(code string, expectedLogin string) error {
	// Ensure App Access Token is set so the one-shot client can fall back
	// for endpoints that allow app auth.
	if _, err := Client(); err != nil {
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
	if AppAccessToken != "" {
		bootstrapClient.SetAppAccessToken(AppAccessToken)
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
	if checkHelixResp("GetUsers", &usersResp.ResponseCommon) {
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
		tokenMu.Lock()
		currentUserToken = tok
		tokenMu.Unlock()
		if currentTwitchClient != nil {
			currentTwitchClient.SetUserAccessToken(tok.AccessToken)
		}
	case c.Conf.ChannelName:
		tokenMu.Lock()
		currentBroadcasterToken = tok
		tokenMu.Unlock()
		if broadcasterTwitchClient != nil {
			broadcasterTwitchClient.SetUserAccessToken(tok.AccessToken)
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
func RefreshUserAccessToken(ctx context.Context) {
	botUser := c.Conf.BotUsername
	broadcasterUser := c.Conf.ChannelName

	refreshOne(ctx, botUser, applyBotToken)

	if broadcasterUser != "" && broadcasterUser != botUser {
		refreshOne(ctx, broadcasterUser, applyBroadcasterToken)
	}
}

// applyBotToken writes the rotated token into the bot slot + primes the
// bot helix client. Passed to refreshOne so the per-identity slot logic
// stays out of the refresh dance itself.
func applyBotToken(tok oauthtokens.Token) {
	tokenMu.Lock()
	currentUserToken = tok
	tokenMu.Unlock()
	if currentTwitchClient != nil {
		currentTwitchClient.SetUserAccessToken(tok.AccessToken)
	}
}

// applyBroadcasterToken writes the rotated token into the broadcaster slot
// + primes the broadcaster helix client.
func applyBroadcasterToken(tok oauthtokens.Token) {
	tokenMu.Lock()
	currentBroadcasterToken = tok
	tokenMu.Unlock()
	if broadcasterTwitchClient != nil {
		broadcasterTwitchClient.SetUserAccessToken(tok.AccessToken)
	}
}

// refreshOne rotates a single (provider="twitch", username) row if within
// 30 minutes of expiry. applyInMemory writes the rotated token into the
// matching in-memory slot — also called on the empty-token path so the
// caller's slot gets blanked (forcing reconnect / re-bootstrap signalling).
//
// TODO: helix client surface should move behind an interface so this
// function can be unit-tested without a real Twitch round-trip.
func refreshOne(ctx context.Context, username string, applyInMemory func(oauthtokens.Token)) {
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

	if time.Until(t.ExpiresAt) > 30*time.Minute {
		return
	}

	client, err := Client()
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
		slog.ErrorContext(ctx, "oauth refresh failed; need re-bootstrap", "err", errors.New("empty access token in refresh response"), "username", username, "reauth_url", AuthInitURL(accountLabel(username)))
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
