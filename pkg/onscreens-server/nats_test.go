package onscreensServer

import (
	"testing"

	"github.com/nats-io/nats.go"
)

// TestHandleMiddleTextShow_DecodesAndShows asserts a well-formed NATS
// message lands on the MiddleText overlay the same way the HTTP handler
// would (s.MiddleText.Show is the shared path).
func TestHandleMiddleTextShow_DecodesAndShows(t *testing.T) {
	s := &Server{MiddleText: newMiddleText()}

	msg := &nats.Msg{
		Subject: "tripbot.test.onscreens.middle.show",
		Data:    []byte(`{"msg":"hello from nats","emitted_at":"2026-05-28T16:00:00Z"}`),
	}
	s.handleMiddleTextShow(msg)

	if !s.MiddleText.IsShowing {
		t.Errorf("MiddleText.IsShowing = false, want true")
	}
	if s.MiddleText.Content != "hello from nats" {
		t.Errorf("MiddleText.Content = %q, want %q", s.MiddleText.Content, "hello from nats")
	}
}

// TestHandleMiddleTextShow_RejectsEmptyMsg covers the defensive check
// for a malformed publisher that omits the msg field. The overlay's
// existing Content must not change. (MiddleText starts IsShowing=true by
// design — it carries pre-restart text — so this asserts on Content.)
func TestHandleMiddleTextShow_RejectsEmptyMsg(t *testing.T) {
	s := &Server{MiddleText: newMiddleText()}
	s.MiddleText.Content = "pre-existing"

	msg := &nats.Msg{
		Subject: "tripbot.test.onscreens.middle.show",
		Data:    []byte(`{"emitted_at":"2026-05-28T16:00:00Z"}`),
	}
	s.handleMiddleTextShow(msg)

	if s.MiddleText.Content != "pre-existing" {
		t.Errorf("MiddleText.Content = %q, want pre-existing (empty msg should be a no-op)", s.MiddleText.Content)
	}
}

// TestHandleMiddleTextShow_RejectsBadJSON covers a non-JSON payload.
func TestHandleMiddleTextShow_RejectsBadJSON(t *testing.T) {
	s := &Server{MiddleText: newMiddleText()}
	s.MiddleText.Content = "pre-existing"

	msg := &nats.Msg{
		Subject: "tripbot.test.onscreens.middle.show",
		Data:    []byte(`not json at all`),
	}
	s.handleMiddleTextShow(msg)

	if s.MiddleText.Content != "pre-existing" {
		t.Errorf("MiddleText.Content = %q, want pre-existing (bad JSON should be a no-op)", s.MiddleText.Content)
	}
}
