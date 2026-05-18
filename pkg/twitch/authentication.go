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
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/oauthtokens"
	"github.com/nicklaw5/helix/v2"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// helixHTTPClient is the otelhttp-instrumented HTTP client passed to every
// helix.NewClient call. Without this, outbound Twitch Helix requests leave
// no trail in Tempo; with it, each helix.GetUsers / GetSubscriptions / etc.
// shows up as a span. Pairs with the otelhttp transports already used by
// pkg/vlc-client and pkg/onscreens-client.
var helixHTTPClient = &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}

// Scopes is the OAuth scope set requested for the bot account. Single source
// of truth — referenced by Client() (App Access Token request),
// GenerateUserAccessToken (initial user-token exchange), and any code that
// builds an authorize URL (cmd/auth-bootstrap, /auth/init).
//
// chat:read + chat:edit are required for IRC. The remaining scopes preserve
// the broadcast + subscription Helix calls the bot already made.
var Scopes = []string{
	"chat:read",
	"chat:edit",
	"channel:read:subscriptions",
	"user:edit:broadcast",
	"moderator:read:chatters",
	"moderator:read:followers",
}

// ErrNoToken signals "no oauth_tokens row for the bot account; run the
// bootstrap CLI." Re-exported from oauthtokens for caller convenience.
var ErrNoToken = oauthtokens.ErrNoToken

// currentTwitchClient is the lazy-initialized helix client.
var currentTwitchClient *helix.Client

// ClientID, ClientSecret are set from env at init.
// AppAccessToken is set in Client() (Client Credentials grant).
var (
	ClientID       string
	ClientSecret   string
	AppAccessToken string
)

// tokenMu guards currentUserToken. RWMutex because reads (IRCAuthToken,
// CurrentUserAccessToken) outnumber writes (LoadFromDB, refresh).
var (
	tokenMu          sync.RWMutex
	currentUserToken oauthtokens.Token
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
		terrors.Log(err, "error creating client")
	}

	resp, err := client.RequestAppAccessToken(Scopes)
	if err != nil {
		terrors.Log(err, "error getting app access token from twitch")
	}
	AppAccessToken = resp.Data.AccessToken
	client.SetAppAccessToken(AppAccessToken)

	currentTwitchClient = client
	return client, err
}

// LoadFromDB pulls the row for the configured bot account into in-memory
// state. cmd/tripbot calls this once at boot before chatbot.Initialize; the
// returned error is checked for ErrNoToken so the cold-start message points
// at the bootstrap CLI.
func LoadFromDB() error {
	t, err := oauthtokens.Get("twitch", c.Conf.BotUsername)
	if err != nil {
		return err
	}
	tokenMu.Lock()
	currentUserToken = t
	tokenMu.Unlock()

	// If the helix client is already built, prime its user-access-token so
	// follow-up Helix calls that need user-scoped permissions work.
	if currentTwitchClient != nil {
		currentTwitchClient.SetUserAccessToken(t.AccessToken)
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

// GenerateUserAccessToken exchanges a code for an access+refresh token,
// derives the authenticated username via helix.GetUsers (so bootstrap is
// account-agnostic — the row is keyed by whoever signed in at consent),
// and Upserts. If the resulting row matches BOT_USERNAME, also primes the
// in-memory state so the running bot picks up the rotated token.
//
// Returns error (no longer swallows). Callers: /auth/callback handler,
// cmd/auth-bootstrap.
func GenerateUserAccessToken(code string) error {
	client, err := Client()
	if err != nil {
		return err
	}

	resp, err := client.RequestUserAccessToken(code)
	if err != nil {
		return err
	}
	if resp == nil || resp.Data.AccessToken == "" {
		return errors.New("twitch: empty access token in code-exchange response")
	}

	// Set the access token so GetUsers identifies the caller from the
	// Authorization header (helix.UsersParams without IDs/Logins returns
	// the authenticated user).
	client.SetUserAccessToken(resp.Data.AccessToken)
	usersResp, err := client.GetUsers(&helix.UsersParams{})
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

	// Bootstrapping a non-bot account (e.g. the broadcaster) writes the row
	// but doesn't change the running bot's IRC session.
	if u.Login == c.Conf.BotUsername {
		tokenMu.Lock()
		currentUserToken = tok
		tokenMu.Unlock()
	}
	return nil
}

// RefreshUserAccessToken is the hourly cron entry point. Reads the bot's row,
// refreshes if within 30 minutes of expiry, and writes the rotated tokens
// back. A Postgres advisory lock fences concurrent rotation between local
// dev and a cluster pod sharing the same Twitch account.
//
// TODO: helix client surface should move behind an interface so this
// function can be unit-tested without a real Twitch round-trip.
func RefreshUserAccessToken(ctx context.Context) {
	botUser := c.Conf.BotUsername

	acquired, release, err := oauthtokens.TryRefreshLock("twitch", botUser)
	if err != nil {
		slog.ErrorContext(ctx, "oauth refresh lock acquisition failed", "err", err)
		return
	}
	if !acquired {
		// Another process is rotating. Wait briefly, re-read the rotated
		// row, sync in-memory.
		time.Sleep(2 * time.Second)
		t, gerr := oauthtokens.Get("twitch", botUser)
		if gerr != nil {
			slog.ErrorContext(ctx, "post-contention re-read failed", "err", gerr)
			return
		}
		tokenMu.Lock()
		currentUserToken = t
		tokenMu.Unlock()
		return
	}
	defer release()

	t, err := oauthtokens.Get("twitch", botUser)
	if err != nil {
		slog.ErrorContext(ctx, "no oauth_tokens row to refresh", "err", err)
		return
	}

	if time.Until(t.ExpiresAt) > 30*time.Minute {
		return
	}

	client, err := Client()
	if err != nil {
		slog.ErrorContext(ctx, "helix client unavailable for refresh", "err", err)
		return
	}
	resp, err := client.RefreshUserAccessToken(t.RefreshToken)
	if err != nil {
		_ = oauthtokens.IncrementFailCount("twitch", botUser)
		slog.ErrorContext(ctx, "twitch refresh API failed", "err", err)
		return
	}
	if resp == nil || resp.Data.AccessToken == "" {
		// Empty body typically means invalid_grant (refresh token revoked).
		// Treat as terminal — blank in-memory so IRC reconnect fails loudly,
		// and Sentry surfaces the failure for re-bootstrap.
		_ = oauthtokens.IncrementFailCount("twitch", botUser)
		tokenMu.Lock()
		currentUserToken.AccessToken = ""
		tokenMu.Unlock()
		slog.ErrorContext(ctx, "oauth refresh failed; need re-bootstrap", "err", errors.New("empty access token in refresh response"))
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
		slog.ErrorContext(ctx, "post-refresh Upsert failed", "err", err)
		return
	}

	tokenMu.Lock()
	currentUserToken = rotated
	tokenMu.Unlock()
	client.SetUserAccessToken(rotated.AccessToken)

	slog.InfoContext(ctx, "refreshed user access token")
}
