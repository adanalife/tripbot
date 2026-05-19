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
