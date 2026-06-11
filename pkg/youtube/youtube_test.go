package youtube

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/oauthtokens"
	"golang.org/x/oauth2"
)

// newTestClient returns a Client with all storage/network seams stubbed to
// fail loudly if a test forgets to override one it exercises.
func newTestClient(t *testing.T) *Client {
	t.Helper()
	return &Client{
		exchange: func(_ context.Context, _ string) (*oauth2.Token, error) {
			t.Fatal("exchange seam not stubbed")
			return nil, nil
		},
		discoverChannel: func(_ context.Context, _ *oauth2.Token) (string, string, error) {
			t.Fatal("discoverChannel seam not stubbed")
			return "", "", nil
		},
		persist: func(_ oauthtokens.Token) error { t.Fatal("persist seam not stubbed"); return nil },
		loadToken: func() (oauthtokens.Token, error) {
			t.Fatal("loadToken seam not stubbed")
			return oauthtokens.Token{}, nil
		},
		recordFailure: func(_, _ string) error { t.Fatal("recordFailure seam not stubbed"); return nil },
	}
}

// withYouTubeConf sets the YouTube config fields for a test and restores
// them on cleanup. c.Conf is the package-level config singleton.
func withYouTubeConf(t *testing.T, clientID, clientSecret, channelID string) {
	t.Helper()
	savedID, savedSecret, savedChannel := c.Conf.YouTubeClientID, c.Conf.YouTubeClientSecret, c.Conf.YouTubeChannelID
	c.Conf.YouTubeClientID = clientID
	c.Conf.YouTubeClientSecret = clientSecret
	c.Conf.YouTubeChannelID = channelID
	t.Cleanup(func() {
		c.Conf.YouTubeClientID, c.Conf.YouTubeClientSecret, c.Conf.YouTubeChannelID = savedID, savedSecret, savedChannel
	})
}

func TestGenerateUserAccessToken_PersistsAndPrimes(t *testing.T) {
	withYouTubeConf(t, "client-id", "client-secret", "")
	cl := newTestClient(t)
	expiry := time.Now().Add(time.Hour)
	cl.exchange = func(_ context.Context, code string) (*oauth2.Token, error) {
		if code != "auth-code" {
			t.Errorf("exchange got code %q", code)
		}
		return &oauth2.Token{AccessToken: "access-1", RefreshToken: "refresh-1", Expiry: expiry}, nil
	}
	cl.discoverChannel = func(_ context.Context, tok *oauth2.Token) (string, string, error) {
		if tok.AccessToken != "access-1" {
			t.Errorf("discoverChannel got token %q", tok.AccessToken)
		}
		return "UC123", "Test Channel", nil
	}
	var persisted oauthtokens.Token
	cl.persist = func(row oauthtokens.Token) error { persisted = row; return nil }

	title, err := cl.GenerateUserAccessToken(context.Background(), "auth-code")
	if err != nil {
		t.Fatalf("GenerateUserAccessToken: %v", err)
	}
	if title != "Test Channel" {
		t.Errorf("title = %q, want Test Channel", title)
	}
	if persisted.Provider != Provider || persisted.Username != "UC123" {
		t.Errorf("persisted row keyed (%q, %q), want (youtube, UC123)", persisted.Provider, persisted.Username)
	}
	if persisted.AccessToken != "access-1" || persisted.RefreshToken != "refresh-1" {
		t.Errorf("persisted tokens = (%q, %q)", persisted.AccessToken, persisted.RefreshToken)
	}
	if got := cl.ChannelID(); got != "UC123" {
		t.Errorf("ChannelID after bootstrap = %q, want UC123", got)
	}
}

func TestGenerateUserAccessToken_NoRefreshTokenRejected(t *testing.T) {
	withYouTubeConf(t, "client-id", "client-secret", "")
	cl := newTestClient(t)
	cl.exchange = func(_ context.Context, _ string) (*oauth2.Token, error) {
		return &oauth2.Token{AccessToken: "access-only"}, nil
	}

	_, err := cl.GenerateUserAccessToken(context.Background(), "auth-code")
	if err == nil || !strings.Contains(err.Error(), "no refresh token") {
		t.Fatalf("expected no-refresh-token error, got %v", err)
	}
}

func TestGenerateUserAccessToken_ChannelMismatchNotPersisted(t *testing.T) {
	withYouTubeConf(t, "client-id", "client-secret", "UCexpected")
	cl := newTestClient(t)
	cl.exchange = func(_ context.Context, _ string) (*oauth2.Token, error) {
		return &oauth2.Token{AccessToken: "access-1", RefreshToken: "refresh-1"}, nil
	}
	cl.discoverChannel = func(_ context.Context, _ *oauth2.Token) (string, string, error) {
		return "UCwrong", "Personal Channel", nil
	}
	// persist stays at the t.Fatal stub: reaching it fails the test, which
	// is exactly the mismatch contract (no row written).

	_, err := cl.GenerateUserAccessToken(context.Background(), "auth-code")
	var mismatch *ErrChannelMismatch
	if !errors.As(err, &mismatch) {
		t.Fatalf("expected ErrChannelMismatch, got %v", err)
	}
	if mismatch.Expected != "UCexpected" || mismatch.Got != "UCwrong" {
		t.Errorf("mismatch = %+v", mismatch)
	}
	if got := cl.ChannelID(); got != "" {
		t.Errorf("in-memory token primed despite mismatch: %q", got)
	}
}

func TestGenerateUserAccessToken_NotConfigured(t *testing.T) {
	withYouTubeConf(t, "", "", "")
	cl := newTestClient(t)
	if _, err := cl.GenerateUserAccessToken(context.Background(), "code"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func TestLoadFromDB_PrimesCurrent(t *testing.T) {
	cl := newTestClient(t)
	cl.loadToken = func() (oauthtokens.Token, error) {
		return oauthtokens.Token{Provider: Provider, Username: "UC123", AccessToken: "a", RefreshToken: "r"}, nil
	}
	if err := cl.LoadFromDB(); err != nil {
		t.Fatalf("LoadFromDB: %v", err)
	}
	if got := cl.ChannelID(); got != "UC123" {
		t.Errorf("ChannelID = %q, want UC123", got)
	}
}

func TestLoadFromDB_NoRowPassesThroughErrNoToken(t *testing.T) {
	cl := newTestClient(t)
	cl.loadToken = func() (oauthtokens.Token, error) { return oauthtokens.Token{}, ErrNoToken }
	if err := cl.LoadFromDB(); !errors.Is(err, ErrNoToken) {
		t.Fatalf("expected ErrNoToken, got %v", err)
	}
}

func TestService_NoTokenBeforeLoad(t *testing.T) {
	withYouTubeConf(t, "client-id", "client-secret", "")
	cl := newTestClient(t)
	if _, err := cl.Service(context.Background()); !errors.Is(err, ErrNoToken) {
		t.Fatalf("expected ErrNoToken, got %v", err)
	}
}

func TestService_NotConfigured(t *testing.T) {
	withYouTubeConf(t, "", "", "")
	cl := newTestClient(t)
	if _, err := cl.Service(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

// staticSource is a fake inner TokenSource for persistingSource tests.
type staticSource struct {
	tok *oauth2.Token
	err error
}

func (s staticSource) Token() (*oauth2.Token, error) { return s.tok, s.err }

func TestPersistingSource_RotationPersisted(t *testing.T) {
	cl := newTestClient(t)
	cl.current = oauthtokens.Token{
		Provider: Provider, Username: "UC123",
		AccessToken: "old-access", RefreshToken: "refresh-1", Scopes: "s",
	}
	var persisted oauthtokens.Token
	cl.persist = func(row oauthtokens.Token) error { persisted = row; return nil }

	expiry := time.Now().Add(time.Hour)
	// Google omits refresh_token on refresh responses — the wrapper must
	// carry the stored one forward.
	ps := &persistingSource{cl: cl, inner: staticSource{tok: &oauth2.Token{AccessToken: "new-access", Expiry: expiry}}}
	tok, err := ps.Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok.AccessToken != "new-access" {
		t.Errorf("returned token = %q", tok.AccessToken)
	}
	if persisted.AccessToken != "new-access" || persisted.RefreshToken != "refresh-1" {
		t.Errorf("persisted = (%q, %q), want (new-access, refresh-1)", persisted.AccessToken, persisted.RefreshToken)
	}
	cl.tokenMu.RLock()
	defer cl.tokenMu.RUnlock()
	if cl.current.AccessToken != "new-access" {
		t.Errorf("in-memory token not rotated: %q", cl.current.AccessToken)
	}
}

func TestPersistingSource_SameTokenNotRePersisted(t *testing.T) {
	cl := newTestClient(t)
	cl.current = oauthtokens.Token{Provider: Provider, Username: "UC123", AccessToken: "access-1", RefreshToken: "r"}
	// persist stays at the t.Fatal stub: calling it on a non-rotation fails the test.

	ps := &persistingSource{cl: cl, inner: staticSource{tok: &oauth2.Token{AccessToken: "access-1"}}}
	if _, err := ps.Token(); err != nil {
		t.Fatalf("Token: %v", err)
	}
}

func TestPersistingSource_RefreshFailureRecorded(t *testing.T) {
	cl := newTestClient(t)
	cl.current = oauthtokens.Token{Provider: Provider, Username: "UC123", AccessToken: "a", RefreshToken: "r"}
	var failedUser string
	cl.recordFailure = func(provider, username string) error {
		if provider != Provider {
			t.Errorf("recordFailure provider = %q", provider)
		}
		failedUser = username
		return nil
	}

	ps := &persistingSource{cl: cl, inner: staticSource{err: errors.New("invalid_grant")}}
	if _, err := ps.Token(); err == nil {
		t.Fatal("expected refresh error to propagate")
	}
	if failedUser != "UC123" {
		t.Errorf("refresh_fail_count not bumped for UC123 (got %q)", failedUser)
	}
}

func TestAuthCodeURL_OfflineWithForcedConsent(t *testing.T) {
	withYouTubeConf(t, "client-id", "client-secret", "")
	cl := newTestClient(t)
	u, err := url.Parse(cl.AuthCodeURL("state-123"))
	if err != nil {
		t.Fatalf("AuthCodeURL unparseable: %v", err)
	}
	q := u.Query()
	if q.Get("client_id") != "client-id" || q.Get("state") != "state-123" {
		t.Errorf("client_id/state = %q/%q", q.Get("client_id"), q.Get("state"))
	}
	if q.Get("access_type") != "offline" {
		t.Errorf("access_type = %q, want offline (refresh token required)", q.Get("access_type"))
	}
	if q.Get("approval_prompt") != "force" && q.Get("prompt") == "" {
		t.Errorf("no forced re-consent param in %q", u.String())
	}
	if !strings.Contains(q.Get("scope"), "youtube.force-ssl") {
		t.Errorf("scope = %q", q.Get("scope"))
	}
}
