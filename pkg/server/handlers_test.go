package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
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
