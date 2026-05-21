package vlcServer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adanalife/tripbot/pkg/httpmw"
)

func TestVersionHandlerReturnsInjectedTag(t *testing.T) {
	s := &Server{Version: "v9.9.9-test"}

	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	rec := httptest.NewRecorder()

	s.versionHandler(rec, req)

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

// These tests exercise *parse-error* paths only — the handlers reject bad
// input before reaching libvlc, so we never touch the real player.

func TestVlcSkipHandlerInvalidIntReturns422(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/vlc/skip?n=notanumber", nil)
	rec := httptest.NewRecorder()

	s.vlcSkipHandler(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestVlcBackHandlerInvalidIntReturns422(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/vlc/back?n=notanumber", nil)
	rec := httptest.NewRecorder()

	s.vlcBackHandler(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestCatchAllHandlerNonGet(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/anything", nil)
	rec := httptest.NewRecorder()

	s.catchAllHandler(rec, req)

	body := rec.Body.String()
	if body == "" {
		t.Fatal("expected non-empty body for non-GET")
	}
}

func TestCatchAllHandlerGet404(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rec := httptest.NewRecorder()

	s.catchAllHandler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// liveness is a process-is-alive signal — must answer OK regardless of
// player state. Exercises the same httpmw helper Start wires in.
func TestLivenessHandlerAlwaysOK(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	rec := httptest.NewRecorder()

	httpmw.LivenessHandler()(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
}

// readiness must fail with 503 when the player isn't initialized. We can't
// easily simulate a *libvlc.Player in a particular MediaState from a unit
// test (would need a real libvlc backend), so this covers the nil-Player
// branch — the same branch Health() returns the "player not initialized"
// error from — through the same httpmw.ReadyCheck wiring Start uses.
func TestReadinessHandlerNilPlayer503(t *testing.T) {
	s := &Server{}
	handler := httpmw.ReadinessHandler(
		httpmw.ReadyCheck{Name: "vlc_player", Fn: func(context.Context) error { return s.Health() }},
	)
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestHealthNilPlayerReturnsError(t *testing.T) {
	s := &Server{}
	if err := s.Health(); err == nil {
		t.Fatal("expected error from Health() with nil Player, got nil")
	}
}
