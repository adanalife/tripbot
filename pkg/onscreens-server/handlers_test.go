package onscreensServer

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

// The leaderboard onscreen is registered with RenderAsHTML, so its render
// HTML must opt the JS into innerHTML (data-html="true") and ship the
// .lb-grid CSS.
func TestRenderLeaderboardEmitsHTMLMode(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/onscreens/render/leaderboard", nil)
	req = mux.SetURLVars(req, map[string]string{"name": SlugLeaderboard})
	rec := httptest.NewRecorder()

	s.onscreensRenderHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-html="true"`) {
		t.Fatalf("expected data-html=\"true\" on leaderboard root, got body:\n%s", body)
	}
	if !strings.Contains(body, ".lb-grid") {
		t.Fatalf("expected .lb-grid CSS in leaderboard render, got body:\n%s", body)
	}
}

// A non-HTML onscreen should keep the legacy textContent path
// (data-html="false") so middle-text et al. don't suddenly start parsing
// chat content as HTML.
func TestRenderMiddleTextStaysTextContent(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/onscreens/render/middle-text", nil)
	req = mux.SetURLVars(req, map[string]string{"name": SlugMiddleText})
	rec := httptest.NewRecorder()

	s.onscreensRenderHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `data-html="false"`) {
		t.Fatalf("expected data-html=\"false\" on middle-text root, got body:\n%s", rec.Body.String())
	}
}
