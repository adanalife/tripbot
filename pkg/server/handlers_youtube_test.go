package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adanalife/tripbot/pkg/server/oauthstate"
	myyoutube "github.com/adanalife/tripbot/pkg/youtube"
)

// withStubYouTubeSeams swaps the YouTube auth seams so the handlers can be
// tested without round-tripping to Google.
func withStubYouTubeSeams(t *testing.T, configured bool, generate func(context.Context, string) (string, error)) {
	t.Helper()
	savedGen, savedURL, savedConf := youtubeGenerateToken, youtubeAuthCodeURL, youtubeConfigured
	youtubeGenerateToken = generate
	youtubeAuthCodeURL = func(state string) string {
		return "https://accounts.google.com/o/oauth2/auth?state=" + state
	}
	youtubeConfigured = func() bool { return configured }
	t.Cleanup(func() {
		youtubeGenerateToken, youtubeAuthCodeURL, youtubeConfigured = savedGen, savedURL, savedConf
	})
}

func TestAuthInitHandler_YouTubeRedirectsToGoogle(t *testing.T) {
	withStubYouTubeSeams(t, true, func(context.Context, string) (string, error) {
		t.Fatal("generator should not be called from /auth/init")
		return "", nil
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/init?account=youtube", nil)
	rec := httptest.NewRecorder()
	authInitHandler(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusFound)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://accounts.google.com/") {
		t.Fatalf("redirect location %q is not Google's authorize URL", loc)
	}
	// The state embedded in the redirect must be valid and carry the
	// youtube account selector for the callback to branch on.
	state := strings.TrimPrefix(loc, "https://accounts.google.com/o/oauth2/auth?state=")
	account, ok := oauthstate.Validate(state)
	if !ok || account != oauthstate.AccountYouTube {
		t.Errorf("state validates to (%q, %v), want (youtube, true)", account, ok)
	}
}

func TestAuthInitHandler_YouTubeUnconfiguredReturns503(t *testing.T) {
	withStubYouTubeSeams(t, false, func(context.Context, string) (string, error) {
		t.Fatal("generator should not be called when unconfigured")
		return "", nil
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/init?account=youtube", nil)
	rec := httptest.NewRecorder()
	authInitHandler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestAuthCallbackHandler_YouTubeHappyPath(t *testing.T) {
	state := oauthstate.New(oauthstate.AccountYouTube)
	var gotCode string
	withStubYouTubeSeams(t, true, func(_ context.Context, code string) (string, error) {
		gotCode = code
		return "Test Channel", nil
	})
	// The Twitch generator must not fire for a youtube-account state.
	withStubGenerateUserAccessToken(t, func(string, string) error {
		t.Fatal("twitch generator called for youtube state")
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+state+"&code=g-code", nil)
	rec := httptest.NewRecorder()
	authCallbackHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	if gotCode != "g-code" {
		t.Errorf("generator got code %q, want g-code", gotCode)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Success") || !strings.Contains(body, "Test Channel") {
		t.Errorf("body should contain Success + channel title; got %q", body)
	}
}

func TestAuthCallbackHandler_YouTubeMismatchReturns400(t *testing.T) {
	state := oauthstate.New(oauthstate.AccountYouTube)
	withStubYouTubeSeams(t, true, func(context.Context, string) (string, error) {
		return "", &myyoutube.ErrChannelMismatch{Expected: "UCexpected", Got: "UCwrong", GotTitle: "Personal"}
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+state+"&code=g-code", nil)
	rec := httptest.NewRecorder()
	authCallbackHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusBadRequest)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Wrong channel") || !strings.Contains(body, "UCexpected") {
		t.Errorf("mismatch body missing expected copy; got %q", body)
	}
}

func TestAuthCallbackHandler_YouTubeGeneratorErrorReturns500(t *testing.T) {
	state := oauthstate.New(oauthstate.AccountYouTube)
	withStubYouTubeSeams(t, true, func(context.Context, string) (string, error) {
		return "", errors.New("google broke")
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+state+"&code=g-code", nil)
	rec := httptest.NewRecorder()
	authCallbackHandler(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
