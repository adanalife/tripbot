package vlcServer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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

// The vlc skip / back / play / random commands moved to NATS (nats.go); their
// HTTP handlers and the parse-error tests that covered them are gone. The
// equivalent decode-error coverage lives in nats_test.go now.

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
// player state. Construct a Server with no Player to confirm.
func TestLivenessHandlerAlwaysOK(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	rec := httptest.NewRecorder()

	s.livenessHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
}

// readiness must fail with 503 when the player isn't initialized. We
// can't easily simulate a *libvlc.Player in a particular MediaState from
// a unit test (would need a real libvlc backend), so this covers the
// nil-Player branch — the same branch Health() returns the "player not
// initialized" error from.
func TestReadinessHandlerNilPlayer503(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()

	s.readinessHandler(rec, req)

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
