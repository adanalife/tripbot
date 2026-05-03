package vlcServer

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
)

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
