package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/server/oauthstate"
)

func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	rec := httptest.NewRecorder()

	healthHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "OK" {
		t.Fatalf("got body %q, want %q", rec.Body.String(), "OK")
	}
}

func TestVersionHandlerReturnsInjectedTag(t *testing.T) {
	saved := versionTag
	defer func() { versionTag = saved }()
	SetVersion("v9.9.9-test")

	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	rec := httptest.NewRecorder()

	versionHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("got Content-Type %q, want %q", got, "application/json")
	}

	var body struct {
		Tag     string `json:"tag"`
		Sha     string `json:"sha"`
		BuiltAt string `json:"built_at"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("couldn't decode response: %v", err)
	}
	if body.Tag != "v9.9.9-test" {
		t.Fatalf("got tag %q, want %q", body.Tag, "v9.9.9-test")
	}
}

func TestWebhooksTwitchHandlerEchoesChallenge(t *testing.T) {
	saved := c.Conf.DisableTwitchWebhooks
	defer func() { c.Conf.DisableTwitchWebhooks = saved }()
	c.Conf.DisableTwitchWebhooks = false

	req := httptest.NewRequest(http.MethodGet, "/webhooks/twitch?hub.challenge=hello-world", nil)
	rec := httptest.NewRecorder()

	webhooksTwitchHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "hello-world" {
		t.Fatalf("got body %q, want %q", rec.Body.String(), "hello-world")
	}
}

func TestWebhooksTwitchHandlerDisabledReturns501(t *testing.T) {
	saved := c.Conf.DisableTwitchWebhooks
	defer func() { c.Conf.DisableTwitchWebhooks = saved }()
	c.Conf.DisableTwitchWebhooks = true

	req := httptest.NewRequest(http.MethodGet, "/webhooks/twitch?hub.challenge=anything", nil)
	rec := httptest.NewRecorder()

	webhooksTwitchHandler(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestWebhooksTwitchHandlerMissingChallengeReturns404(t *testing.T) {
	saved := c.Conf.DisableTwitchWebhooks
	defer func() { c.Conf.DisableTwitchWebhooks = saved }()
	c.Conf.DisableTwitchWebhooks = false

	req := httptest.NewRequest(http.MethodGet, "/webhooks/twitch", nil)
	rec := httptest.NewRecorder()

	webhooksTwitchHandler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestAuthTwitchHandlerInvalidSecretReturns404(t *testing.T) {
	saved := c.Conf.TripbotHttpAuth
	defer func() { c.Conf.TripbotHttpAuth = saved }()
	c.Conf.TripbotHttpAuth = "secret"

	req := httptest.NewRequest(http.MethodGet, "/auth/twitch?auth=wrong", nil)
	rec := httptest.NewRecorder()

	authTwitchHandler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestAuthTwitchHandlerMissingSecretReturns404(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/auth/twitch", nil)
	rec := httptest.NewRecorder()

	authTwitchHandler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// withStubGenerateUserAccessToken swaps the package-level generator so the
// /auth/callback handler can be tested without round-tripping to Twitch.
func withStubGenerateUserAccessToken(t *testing.T, stub func(string) error) {
	t.Helper()
	saved := generateUserAccessToken
	generateUserAccessToken = stub
	t.Cleanup(func() { generateUserAccessToken = saved })
}

func TestAuthCallbackHandler_NoStateReturns400(t *testing.T) {
	withStubGenerateUserAccessToken(t, func(string) error {
		t.Fatal("generator should not be called when state is missing")
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=anything", nil)
	rec := httptest.NewRecorder()
	authCallbackHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAuthCallbackHandler_BadStateReturns400(t *testing.T) {
	withStubGenerateUserAccessToken(t, func(string) error {
		t.Fatal("generator should not be called when state is invalid")
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/callback?state=not-real&code=anything", nil)
	rec := httptest.NewRecorder()
	authCallbackHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAuthCallbackHandler_NoCodeReturns400(t *testing.T) {
	state := oauthstate.New()
	withStubGenerateUserAccessToken(t, func(string) error {
		t.Fatal("generator should not be called when code is missing")
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+state, nil)
	rec := httptest.NewRecorder()
	authCallbackHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAuthCallbackHandler_HappyPath(t *testing.T) {
	state := oauthstate.New()
	var gotCode string
	withStubGenerateUserAccessToken(t, func(code string) error {
		gotCode = code
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+state+"&code=the-code", nil)
	rec := httptest.NewRecorder()
	authCallbackHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	if gotCode != "the-code" {
		t.Errorf("generator got code %q, want %q", gotCode, "the-code")
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		t.Errorf("Content-Type %q is not html", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "Success") {
		t.Errorf("body should contain 'Success'; got %q", rec.Body.String())
	}
}

func TestAuthCallbackHandler_GeneratorErrorReturns500(t *testing.T) {
	state := oauthstate.New()
	withStubGenerateUserAccessToken(t, func(string) error {
		return errors.New("twitch broke")
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+state+"&code=anything", nil)
	rec := httptest.NewRecorder()
	authCallbackHandler(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestAuthCallbackHandler_StateIsSingleUse(t *testing.T) {
	state := oauthstate.New()
	withStubGenerateUserAccessToken(t, func(string) error { return nil })

	// First call consumes the state and succeeds.
	req1 := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+state+"&code=x", nil)
	rec1 := httptest.NewRecorder()
	authCallbackHandler(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first call: got status %d, want %d", rec1.Code, http.StatusOK)
	}

	// Second call with the same state should 400.
	req2 := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+state+"&code=x", nil)
	rec2 := httptest.NewRecorder()
	authCallbackHandler(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("second call: got status %d, want %d (state should be single-use)", rec2.Code, http.StatusBadRequest)
	}
}

func TestCatchAllHandlerGet404(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rec := httptest.NewRecorder()

	catchAllHandler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestCatchAllHandlerPost404(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/missing", nil)
	rec := httptest.NewRecorder()

	catchAllHandler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusNotFound)
	}
}
