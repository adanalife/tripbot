package vlcServer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
)

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

// These tests exercise *parse-error* paths only — the handlers reject bad
// input before reaching libvlc, so we never touch the real player.

func TestVlcSkipHandlerInvalidIntReturns422(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/vlc/skip?n=notanumber", nil)
	rec := httptest.NewRecorder()

	vlcSkipHandler(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestVlcBackHandlerInvalidIntReturns422(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/vlc/back?n=notanumber", nil)
	rec := httptest.NewRecorder()

	vlcBackHandler(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestOnscreensMiddleHandlerInvalidBase64Returns422(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/onscreens/middle/show?msg=!!!not-base64!!!", nil)
	req = mux.SetURLVars(req, map[string]string{"action": "show"})
	rec := httptest.NewRecorder()

	onscreensMiddleHandler(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestOnscreensMiddleHandlerMissingMsgReturns417(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/onscreens/middle/show", nil)
	req = mux.SetURLVars(req, map[string]string{"action": "show"})
	rec := httptest.NewRecorder()

	onscreensMiddleHandler(rec, req)

	if rec.Code != http.StatusExpectationFailed {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusExpectationFailed)
	}
}

func TestOnscreensMiddleHandlerUnknownActionReturns417(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/onscreens/middle/explode", nil)
	req = mux.SetURLVars(req, map[string]string{"action": "explode"})
	rec := httptest.NewRecorder()

	onscreensMiddleHandler(rec, req)

	if rec.Code != http.StatusExpectationFailed {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusExpectationFailed)
	}
}

func TestOnscreensLeaderboardHandlerInvalidBase64Returns422(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/onscreens/leaderboard/show?content=!!!", nil)
	req = mux.SetURLVars(req, map[string]string{"action": "show"})
	rec := httptest.NewRecorder()

	onscreensLeaderboardHandler(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestOnscreensFlagHandlerUnknownActionReturns417(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/onscreens/flag/explode", nil)
	req = mux.SetURLVars(req, map[string]string{"action": "explode"})
	rec := httptest.NewRecorder()

	onscreensFlagHandler(rec, req)

	if rec.Code != http.StatusExpectationFailed {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusExpectationFailed)
	}
}

func TestCatchAllHandlerNonGet(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/anything", nil)
	rec := httptest.NewRecorder()

	catchAllHandler(rec, req)

	body := rec.Body.String()
	if body == "" {
		t.Fatal("expected non-empty body for non-GET")
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
