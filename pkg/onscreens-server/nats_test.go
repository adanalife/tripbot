package onscreensServer

import (
	"strings"
	"testing"

	"github.com/nats-io/nats.go"
)

// TestHandleMiddleShow_DecodesAndShows asserts a well-formed NATS message
// lands on the MiddleText overlay the same way the HTTP handler would
// (s.MiddleText.Show is the shared path).
func TestHandleMiddleShow_DecodesAndShows(t *testing.T) {
	s := &Server{MiddleText: newMiddleText()}

	msg := &nats.Msg{
		Subject: "tripbot.test.onscreens.middle.show",
		Data:    []byte(`{"msg":"hello from nats","emitted_at":"2026-05-28T16:00:00Z"}`),
	}
	s.handleMiddleShow(msg)

	if !s.MiddleText.IsShowing {
		t.Errorf("MiddleText.IsShowing = false, want true")
	}
	if s.MiddleText.Content != "hello from nats" {
		t.Errorf("MiddleText.Content = %q, want %q", s.MiddleText.Content, "hello from nats")
	}
}

// TestHandleMiddleShow_RejectsEmptyMsg covers the defensive check for a
// malformed publisher that omits the msg field. The overlay's existing
// Content must not change. (MiddleText starts IsShowing=true by design — it
// carries pre-restart text — so this asserts on Content.)
func TestHandleMiddleShow_RejectsEmptyMsg(t *testing.T) {
	s := &Server{MiddleText: newMiddleText()}
	s.MiddleText.Content = "pre-existing"

	msg := &nats.Msg{
		Subject: "tripbot.test.onscreens.middle.show",
		Data:    []byte(`{"emitted_at":"2026-05-28T16:00:00Z"}`),
	}
	s.handleMiddleShow(msg)

	if s.MiddleText.Content != "pre-existing" {
		t.Errorf("MiddleText.Content = %q, want pre-existing (empty msg should be a no-op)", s.MiddleText.Content)
	}
}

// TestHandleMiddleShow_RejectsBadJSON covers a non-JSON payload.
func TestHandleMiddleShow_RejectsBadJSON(t *testing.T) {
	s := &Server{MiddleText: newMiddleText()}
	s.MiddleText.Content = "pre-existing"

	msg := &nats.Msg{
		Subject: "tripbot.test.onscreens.middle.show",
		Data:    []byte(`not json at all`),
	}
	s.handleMiddleShow(msg)

	if s.MiddleText.Content != "pre-existing" {
		t.Errorf("MiddleText.Content = %q, want pre-existing (bad JSON should be a no-op)", s.MiddleText.Content)
	}
}

// emptyMsg is an envelope-only payload — the shape every hide + the
// empty-payload shows arrive as.
func emptyMsg(subject string) *nats.Msg {
	return &nats.Msg{Subject: subject, Data: []byte(`{"emitted_at":"2026-05-28T16:00:00Z"}`)}
}

func TestHandleMiddleHide(t *testing.T) {
	s := &Server{MiddleText: newMiddleText()}
	s.MiddleText.Show("something")
	s.handleMiddleHide(emptyMsg("tripbot.test.onscreens.middle.hide"))
	if s.MiddleText.IsShowing {
		t.Error("MiddleText.IsShowing = true, want false after hide")
	}
}

func TestHandleLeaderboardShow(t *testing.T) {
	s := &Server{Leaderboard: newLeaderboardOnscreen()}

	msg := &nats.Msg{
		Subject: "tripbot.test.onscreens.leaderboard.show",
		Data:    []byte(`{"emitted_at":"2026-05-28T16:00:00Z","title":"monthly miles","rows":[["alice","100"]]}`),
	}
	s.handleLeaderboardShow(msg)

	if !s.Leaderboard.IsShowing {
		t.Error("Leaderboard.IsShowing = false, want true")
	}
	// Server renders the HTML from {title, rows}.
	if !strings.Contains(s.Leaderboard.Content, `<div class="lb-title">Monthly Miles</div>`) {
		t.Errorf("Leaderboard.Content missing rendered title, got %q", s.Leaderboard.Content)
	}
	if !strings.Contains(s.Leaderboard.Content, "(alice)") {
		t.Errorf("Leaderboard.Content missing user, got %q", s.Leaderboard.Content)
	}
}

func TestHandleLeaderboardShow_RejectsBadJSON(t *testing.T) {
	s := &Server{Leaderboard: newLeaderboardOnscreen()}
	s.Leaderboard.Content = "pre-existing"

	s.handleLeaderboardShow(&nats.Msg{
		Subject: "tripbot.test.onscreens.leaderboard.show",
		Data:    []byte(`not json`),
	})

	if s.Leaderboard.Content != "pre-existing" {
		t.Errorf("Leaderboard.Content = %q, want pre-existing (bad JSON should be a no-op)", s.Leaderboard.Content)
	}
}

func TestHandleLeaderboardHide(t *testing.T) {
	s := &Server{Leaderboard: newLeaderboardOnscreen()}
	s.Leaderboard.ShowFor("x", leaderboardDuration)
	s.handleLeaderboardHide(emptyMsg("tripbot.test.onscreens.leaderboard.hide"))
	if s.Leaderboard.IsShowing {
		t.Error("Leaderboard.IsShowing = true, want false after hide")
	}
}

func TestHandleTimewarpShow(t *testing.T) {
	s := &Server{Timewarp: newTimewarp()}
	s.handleTimewarpShow(emptyMsg("tripbot.test.onscreens.timewarp.show"))
	if !s.Timewarp.IsShowing {
		t.Error("Timewarp.IsShowing = false, want true")
	}
	if s.Timewarp.Content != "Timewarp!" {
		t.Errorf("Timewarp.Content = %q, want Timewarp!", s.Timewarp.Content)
	}
}

func TestHandleTimewarpHide(t *testing.T) {
	s := &Server{Timewarp: newTimewarp()}
	s.Timewarp.ShowFor("Timewarp!", timewarpDuration)
	s.handleTimewarpHide(emptyMsg("tripbot.test.onscreens.timewarp.hide"))
	if s.Timewarp.IsShowing {
		t.Error("Timewarp.IsShowing = true, want false after hide")
	}
}

func TestHandleGPSShowHide(t *testing.T) {
	s := &Server{GPS: newGPSOnscreen()}
	s.handleGPSShow(emptyMsg("tripbot.test.onscreens.gps.show"))
	if !s.GPS.IsShowing {
		t.Error("GPS.IsShowing = false, want true after show")
	}
	s.handleGPSHide(emptyMsg("tripbot.test.onscreens.gps.hide"))
	if s.GPS.IsShowing {
		t.Error("GPS.IsShowing = true, want false after hide")
	}
}

func TestHandleFlagHide(t *testing.T) {
	s := &Server{Flag: newFlagOnscreen()}
	s.Flag.Show("")
	s.handleFlagHide(emptyMsg("tripbot.test.onscreens.flag.hide"))
	if s.Flag.IsShowing {
		t.Error("Flag.IsShowing = true, want false after hide")
	}
}

// TestHideLenientOnEmptyBody asserts a hide with a nil/garbage body still
// hides — the subject is the whole intent.
func TestHideLenientOnEmptyBody(t *testing.T) {
	s := &Server{MiddleText: newMiddleText()}
	s.MiddleText.Show("x")
	s.handleMiddleHide(&nats.Msg{Subject: "tripbot.test.onscreens.middle.hide", Data: nil})
	if s.MiddleText.IsShowing {
		t.Error("hide should act regardless of body")
	}
}
