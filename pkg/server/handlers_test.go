package server

import (
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
