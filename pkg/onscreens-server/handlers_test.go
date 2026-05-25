package onscreensServer

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
)

// These tests exercise *parse-error* paths only — the handlers reject bad
// input before reaching the onscreen singletons.

func TestOnscreensMiddleHandlerInvalidBase64Returns422(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/onscreens/middle/show?msg=!!!not-base64!!!", nil)
	req = mux.SetURLVars(req, map[string]string{"action": "show"})
	rec := httptest.NewRecorder()

	s.onscreensMiddleHandler(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestOnscreensMiddleHandlerMissingMsgReturns417(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/onscreens/middle/show", nil)
	req = mux.SetURLVars(req, map[string]string{"action": "show"})
	rec := httptest.NewRecorder()

	s.onscreensMiddleHandler(rec, req)

	if rec.Code != http.StatusExpectationFailed {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusExpectationFailed)
	}
}

func TestOnscreensMiddleHandlerUnknownActionReturns417(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/onscreens/middle/explode", nil)
	req = mux.SetURLVars(req, map[string]string{"action": "explode"})
	rec := httptest.NewRecorder()

	s.onscreensMiddleHandler(rec, req)

	if rec.Code != http.StatusExpectationFailed {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusExpectationFailed)
	}
}

func TestOnscreensLeaderboardHandlerInvalidBase64Returns422(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/onscreens/leaderboard/show?content=!!!", nil)
	req = mux.SetURLVars(req, map[string]string{"action": "show"})
	rec := httptest.NewRecorder()

	s.onscreensLeaderboardHandler(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestOnscreensFlagHandlerUnknownActionReturns417(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/onscreens/flag/explode", nil)
	req = mux.SetURLVars(req, map[string]string{"action": "explode"})
	rec := httptest.NewRecorder()

	s.onscreensFlagHandler(rec, req)

	if rec.Code != http.StatusExpectationFailed {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusExpectationFailed)
	}
}
