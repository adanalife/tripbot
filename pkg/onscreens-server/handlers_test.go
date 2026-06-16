package onscreensServer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

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

// Middle-text renders inline markdown, so its browser source must opt into
// innerHTML (data-html="true") and ship the monospace `code` styling.
func TestRenderMiddleTextEmitsHTMLMode(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/onscreens/render/middle-text", nil)
	req = mux.SetURLVars(req, map[string]string{"name": SlugMiddleText})
	rec := httptest.NewRecorder()

	s.onscreensRenderHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-html="true"`) {
		t.Fatalf("expected data-html=\"true\" on middle-text root, got body:\n%s", body)
	}
	if !strings.Contains(body, "#root code") {
		t.Fatalf("expected #root code CSS in middle-text render, got body:\n%s", body)
	}
}

// state.json renders the markdown source of a Markdown-flagged onscreen to
// HTML at the wire boundary, while the stored Content stays the raw source.
func TestStateHandlerRendersMarkdown(t *testing.T) {
	s := newTestServer(t)
	s.MiddleText.Show("use `!find` to search")

	req := httptest.NewRequest(http.MethodGet, "/onscreens/state.json", nil)
	rec := httptest.NewRecorder()
	s.onscreensStateHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	// Decode the wire JSON (the encoder \u-escapes '<'/'>'; the browser's
	// JSON.parse decodes them back to real tags before innerHTML).
	var got map[string]struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding state.json: %v", err)
	}
	if want := "use <code>!find</code> to search"; got[SlugMiddleText].Content != want {
		t.Fatalf("middle-text wire content = %q, want %q", got[SlugMiddleText].Content, want)
	}
	// Stored Content stays untouched raw markdown.
	if c := s.MiddleText.Content; c != "use `!find` to search" {
		t.Fatalf("stored Content was mutated: %q", c)
	}
}
