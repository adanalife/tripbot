// Package youtube owns tripbot's YouTube identity: the channel-owner OAuth
// token (stored in oauth_tokens, provider="youtube") and the authenticated
// Data-API service client the chat phases (B2 outbound / B3 inbound) call
// through. It mirrors pkg/twitch's role for the Twitch identity.
//
// Identity model: YouTube has exactly one identity — the channel owner.
// Live-chat read/write must run as the channel owner via OAuth user consent
// (service accounts can't operate a channel's live chat), so there is no
// bot/broadcaster split. The oauth_tokens row is keyed by the channel ID,
// discovered at consent time via channels.list(mine=true).
//
// The OAuth client in the GCP console must be a "Web application" client
// with <EXTERNAL_URL>/auth/callback in its authorized redirect URIs — unlike
// cmd/youtube-chat-spike's Desktop client, which only allows localhost.
//
// Token refresh: Google access tokens live ~1h and refresh lazily through
// the oauth2 TokenSource on use; the persisting wrapper writes each rotation
// back to oauth_tokens so a restart picks up where the process left off.
// There is no hourly refresh cron analog — the inbound poller exercises the
// TokenSource constantly, which is what keeps the token warm.
//
// Credentials are optional process-wide: a PLATFORM=twitch instance runs the
// same binary without YOUTUBE_* set, so nothing here log.Fatals at init.
package youtube

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/oauthtokens"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	ytapi "google.golang.org/api/youtube/v3"
)

// Provider is the oauth_tokens provider discriminator for YouTube rows.
const Provider = "youtube"

// Scopes is the OAuth scope set requested at consent. youtube.force-ssl is
// the one scope covering both liveChatMessages.list and .insert.
var Scopes = []string{"https://www.googleapis.com/auth/youtube.force-ssl"}

// ErrNoToken signals "no oauth_tokens row for the youtube provider; run the
// /auth/init?account=youtube flow." Re-exported for caller convenience.
var ErrNoToken = oauthtokens.ErrNoToken

// ErrNotConfigured is returned when the OAuth app credentials are absent —
// expected on non-YouTube instances, an env-wiring bug on YouTube ones.
var ErrNotConfigured = errors.New("youtube: YOUTUBE_CLIENT_ID / YOUTUBE_CLIENT_SECRET not set")

// ErrChannelMismatch is returned by GenerateUserAccessToken when the
// discovered channel doesn't match the configured YOUTUBE_CHANNEL_ID — the
// "consented with the wrong Google account" case (e.g. the personal channel,
// or the quiet test channel against a prod pod). No row is written.
type ErrChannelMismatch struct {
	Expected string // configured YOUTUBE_CHANNEL_ID
	Got      string // discovered channel ID
	GotTitle string // discovered channel title, for the operator-facing message
}

func (e *ErrChannelMismatch) Error() string {
	return fmt.Sprintf("youtube: consent identity mismatch — expected channel %q but signed in as %q (%s). Sign in with the channel-owner Google account and retry.",
		e.Expected, e.Got, e.GotTitle)
}

// Client holds the YouTube identity state: the current oauth_tokens row and
// the lazily-built Data-API service. The seam fields default to the real
// implementations in New(); tests swap them for fakes.
type Client struct {
	tokenMu sync.RWMutex
	current oauthtokens.Token

	svcMu sync.Mutex
	svc   *ytapi.Service

	// exchange swaps an authorization code for a token. Defaults to the
	// oauth2 Config's Exchange; tests stub it.
	exchange func(ctx context.Context, code string) (*oauth2.Token, error)
	// discoverChannel identifies the consenting channel (id, title) from a
	// fresh token. Defaults to channels.list(mine=true); tests stub it.
	discoverChannel func(ctx context.Context, tok *oauth2.Token) (id, title string, err error)
	// persist / loadToken / recordFailure are the oauth_tokens storage seam.
	persist       func(t oauthtokens.Token) error
	loadToken     func() (oauthtokens.Token, error)
	recordFailure func(provider, username string) error
}

// New constructs a Client wired with the production storage + Google
// endpoints. Construction touches no network or DB.
func New() *Client {
	cl := &Client{}
	cl.exchange = func(ctx context.Context, code string) (*oauth2.Token, error) {
		return cl.oauthConfig().Exchange(ctx, code)
	}
	cl.discoverChannel = discoverOwnChannel
	cl.persist = oauthtokens.Upsert
	cl.loadToken = loadFromOauthTokens
	cl.recordFailure = oauthtokens.IncrementFailCount
	return cl
}

// Configured reports whether the OAuth app credentials are present. Handlers
// gate the youtube auth flow on this so Twitch-only deployments 503 cleanly
// instead of redirecting to Google with an empty client_id.
func Configured() bool {
	return c.Conf.YouTubeClientID != "" && c.Conf.YouTubeClientSecret != ""
}

// oauthConfig builds the oauth2 app config. Constructed per-call (not cached)
// so tests that mutate c.Conf see the change; it's just struct assembly.
func (cl *Client) oauthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     c.Conf.YouTubeClientID,
		ClientSecret: c.Conf.YouTubeClientSecret,
		Endpoint:     google.Endpoint,
		// Same callback path as Twitch — the oauthstate account selector
		// disambiguates which IdP a callback belongs to. Must be registered
		// on the GCP OAuth client.
		RedirectURL: c.Conf.ExternalURL + "/auth/callback",
		Scopes:      Scopes,
	}
}

// AuthCodeURL returns Google's authorize URL for the given CSRF state.
// AccessTypeOffline asks for a refresh token; ApprovalForce makes Google
// re-issue one even when consent was already granted (otherwise repeat
// logins return only an access token and the stored refresh token goes
// un-rotated).
func (cl *Client) AuthCodeURL(state string) string {
	return cl.oauthConfig().AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
}

// AuthInitURL returns the operator-facing re-auth URL, mirroring
// mytwitch.AuthInitURL — surfaced in logs instead of the fully-formed Google
// URL, which embeds a short-TTL single-use state.
func AuthInitURL() string {
	return c.Conf.ExternalURL + "/auth/init?account=youtube"
}

// loadFromOauthTokens reads the youtube row. When YOUTUBE_CHANNEL_ID pins the
// identity, read exactly that row; otherwise take the provider's only row
// (GetByProvider — newest wins if strays exist).
func loadFromOauthTokens() (oauthtokens.Token, error) {
	if id := c.Conf.YouTubeChannelID; id != "" {
		return oauthtokens.Get(Provider, id)
	}
	return oauthtokens.GetByProvider(Provider)
}

// LoadFromDB loads the channel-owner row into memory, ready for Service().
// Returns ErrNoToken when the row is missing — callers surface AuthInitURL.
func (cl *Client) LoadFromDB() error {
	t, err := cl.loadToken()
	if err != nil {
		return err
	}
	cl.tokenMu.Lock()
	cl.current = t
	cl.tokenMu.Unlock()
	cl.resetService()
	return nil
}

// ChannelID returns the loaded identity's channel ID, or "" before LoadFromDB.
func (cl *Client) ChannelID() string {
	cl.tokenMu.RLock()
	defer cl.tokenMu.RUnlock()
	return cl.current.Username
}

// GenerateUserAccessToken exchanges an authorization code for tokens,
// discovers the consenting channel, sanity-checks it against
// YOUTUBE_CHANNEL_ID (when set), persists the row, and primes the in-memory
// state so the running pod picks up the new token without a restart. Returns
// the channel title for the success page.
//
// The identity check runs BEFORE persist so a wrong-account consent doesn't
// pollute oauth_tokens — same contract as the Twitch flow's mismatch check.
func (cl *Client) GenerateUserAccessToken(ctx context.Context, code string) (string, error) {
	if !Configured() {
		return "", ErrNotConfigured
	}
	tok, err := cl.exchange(ctx, code)
	if err != nil {
		return "", fmt.Errorf("youtube: code exchange: %w", err)
	}
	if tok.RefreshToken == "" {
		// ApprovalForce should always yield one; absence means the flow was
		// initiated without it (or a Google-side change worth knowing about).
		return "", errors.New("youtube: no refresh token in code-exchange response")
	}

	id, title, err := cl.discoverChannel(ctx, tok)
	if err != nil {
		return "", fmt.Errorf("youtube: channel discovery: %w", err)
	}
	if expected := c.Conf.YouTubeChannelID; expected != "" && id != expected {
		return "", &ErrChannelMismatch{Expected: expected, Got: id, GotTitle: title}
	}

	row := oauthtokens.Token{
		Provider:     Provider,
		Username:     id,
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		ExpiresAt:    tok.Expiry,
		Scopes:       strings.Join(Scopes, " "),
	}
	if err := cl.persist(row); err != nil {
		return "", err
	}
	cl.tokenMu.Lock()
	cl.current = row
	cl.tokenMu.Unlock()
	cl.resetService()
	slog.InfoContext(ctx, "youtube oauth bootstrap complete", "channel", title, "channel_id", id)
	return title, nil
}

// discoverOwnChannel identifies the consenting account's channel via
// channels.list(mine=true) using a one-shot service, so the cached service's
// token source is never touched mid-bootstrap.
func discoverOwnChannel(ctx context.Context, tok *oauth2.Token) (string, string, error) {
	svc, err := ytapi.NewService(ctx, option.WithTokenSource(oauth2.StaticTokenSource(tok)))
	if err != nil {
		return "", "", err
	}
	resp, err := svc.Channels.List([]string{"snippet"}).Mine(true).Do()
	if err != nil {
		return "", "", err
	}
	if len(resp.Items) == 0 {
		return "", "", errors.New("no channel on the consenting Google account")
	}
	ch := resp.Items[0]
	return ch.Id, ch.Snippet.Title, nil
}

// Service returns the authenticated Data-API client, lazy-building it on
// first call. The token source auto-refreshes ~1h access tokens on use and
// persists each rotation (persistingSource). ErrNoToken before LoadFromDB /
// the consent flow has produced a row.
//
// ctx is captured by the underlying HTTP client for the service's lifetime —
// callers pass their long-lived runtime context, not a per-request one.
func (cl *Client) Service(ctx context.Context) (*ytapi.Service, error) {
	if !Configured() {
		return nil, ErrNotConfigured
	}
	cl.svcMu.Lock()
	defer cl.svcMu.Unlock()
	if cl.svc != nil {
		return cl.svc, nil
	}

	cl.tokenMu.RLock()
	t := cl.current
	cl.tokenMu.RUnlock()
	if t.RefreshToken == "" {
		return nil, ErrNoToken
	}

	base := &oauth2.Token{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		Expiry:       t.ExpiresAt,
	}
	// ReuseTokenSource serves base until expiry, then falls through to the
	// persisting wrapper around the real refresher exactly once per rotation.
	ts := oauth2.ReuseTokenSource(base, &persistingSource{
		cl:    cl,
		inner: cl.oauthConfig().TokenSource(ctx, base),
	})
	svc, err := ytapi.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, err
	}
	cl.svc = svc
	return svc, nil
}

// resetService drops the cached service so the next Service() call rebuilds
// its token source from the freshly-primed current token.
func (cl *Client) resetService() {
	cl.svcMu.Lock()
	cl.svc = nil
	cl.svcMu.Unlock()
}

// persistingSource wraps the refreshing TokenSource so every rotation is
// written back to oauth_tokens — a restart then resumes from the stored
// access token instead of burning a refresh on every boot.
type persistingSource struct {
	cl    *Client
	inner oauth2.TokenSource
}

func (p *persistingSource) Token() (*oauth2.Token, error) {
	tok, err := p.inner.Token()
	if err != nil {
		// Surface persistent refresh failures in monitoring via
		// refresh_fail_count, same as the Twitch refresh loop.
		p.cl.tokenMu.RLock()
		username := p.cl.current.Username
		p.cl.tokenMu.RUnlock()
		if username != "" {
			_ = p.cl.recordFailure(Provider, username)
		}
		return nil, err
	}

	p.cl.tokenMu.Lock()
	cur := p.cl.current
	if tok.AccessToken == cur.AccessToken {
		p.cl.tokenMu.Unlock()
		return tok, nil
	}
	// Google usually omits refresh_token from refresh responses; carry the
	// stored one forward so the row never loses it.
	rt := tok.RefreshToken
	if rt == "" {
		rt = cur.RefreshToken
	}
	rotated := oauthtokens.Token{
		Provider:     Provider,
		Username:     cur.Username,
		AccessToken:  tok.AccessToken,
		RefreshToken: rt,
		ExpiresAt:    tok.Expiry,
		Scopes:       cur.Scopes,
	}
	p.cl.current = rotated
	p.cl.tokenMu.Unlock()

	// Persist outside the lock; failure is non-fatal — the in-memory token
	// is valid, we just lose the rotation on restart.
	if err := p.cl.persist(rotated); err != nil {
		slog.Error("youtube: persisting rotated token failed", "err", err, "channel_id", rotated.Username)
	}
	return tok, nil
}
