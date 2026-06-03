package onscreensClient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	oe "github.com/adanalife/tripbot/pkg/onscreens-events"
)

// recordingPublisher captures every publish so tests can assert on the
// subject + payload. Goroutine-safe. Satisfies natsclient.Publisher.
type recordingPublisher struct {
	mu        sync.Mutex
	Publishes []recordedPublish
}

type recordedPublish struct {
	Subject string
	Payload []byte
}

func (r *recordingPublisher) Publish(_ context.Context, subject string, payload []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]byte, len(payload))
	copy(cp, payload)
	r.Publishes = append(r.Publishes, recordedPublish{Subject: subject, Payload: cp})
}

// okServer stands up an httptest.Server that 200s everything, so the
// client's HTTP path succeeds and tests can focus on the NATS mirror.
func okServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

// TestShowMiddleText_PublishesToNATS asserts the client fires the right
// subject + envelope on every ShowMiddleText, alongside the HTTP call.
func TestShowMiddleText_PublishesToNATS(t *testing.T) {
	rec := &recordingPublisher{}
	c := New(okServer(t), rec, "stage")

	if err := c.ShowMiddleText(context.Background(), "hello world"); err != nil {
		t.Fatalf("ShowMiddleText: %v", err)
	}

	if len(rec.Publishes) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(rec.Publishes))
	}
	pub := rec.Publishes[0]
	if pub.Subject != "tripbot.stage.onscreens.middle.show" {
		t.Errorf("subject = %q, want tripbot.stage.onscreens.middle.show", pub.Subject)
	}

	var ev oe.MiddleShow
	if err := json.Unmarshal(pub.Payload, &ev); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if ev.Msg != "hello world" {
		t.Errorf("msg = %q, want hello world", ev.Msg)
	}
	if ev.EmittedAt == "" {
		t.Error("emitted_at empty")
	}
}

// TestShowMiddleText_TopicReflectsEnv covers the subject scoping per env.
func TestShowMiddleText_TopicReflectsEnv(t *testing.T) {
	host := okServer(t)
	for _, env := range []string{"prod", "development", "test"} {
		t.Run(env, func(t *testing.T) {
			rec := &recordingPublisher{}
			c := New(host, rec, env)
			if err := c.ShowMiddleText(context.Background(), "x"); err != nil {
				t.Fatalf("ShowMiddleText: %v", err)
			}
			if len(rec.Publishes) != 1 {
				t.Fatalf("expected 1 publish")
			}
			want := "tripbot." + env + ".onscreens.middle.show"
			if rec.Publishes[0].Subject != want {
				t.Errorf("subject = %q, want %q", rec.Publishes[0].Subject, want)
			}
		})
	}
}

// TestShowLeaderboard_PublishesStructured asserts the leaderboard publish
// carries the structured {title, rows} payload (the server renders it).
func TestShowLeaderboard_PublishesToNATS(t *testing.T) {
	rec := &recordingPublisher{}
	c := New(okServer(t), rec, "prod")

	rows := [][]string{{"alice", "100"}, {"bob", "50"}}
	if err := c.ShowLeaderboard(context.Background(), "Monthly Miles", rows); err != nil {
		t.Fatalf("ShowLeaderboard: %v", err)
	}

	if len(rec.Publishes) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(rec.Publishes))
	}
	pub := rec.Publishes[0]
	if pub.Subject != "tripbot.prod.onscreens.leaderboard.show" {
		t.Errorf("subject = %q, want tripbot.prod.onscreens.leaderboard.show", pub.Subject)
	}

	var ev oe.LeaderboardShow
	if err := json.Unmarshal(pub.Payload, &ev); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if ev.Title != "Monthly Miles" {
		t.Errorf("title = %q, want Monthly Miles", ev.Title)
	}
	if len(ev.Rows) != 2 || ev.Rows[0][0] != "alice" || ev.Rows[1][1] != "50" {
		t.Errorf("rows = %v, want [[alice 100] [bob 50]]", ev.Rows)
	}
}

// TestEmptyPayloadCommandsPublish covers the mirror for the no-payload
// commands: each fires exactly one publish on its subject with an envelope
// that carries emitted_at.
func TestEmptyPayloadCommandsPublish(t *testing.T) {
	host := okServer(t)
	cases := []struct {
		name    string
		call    func(c *Client) error
		subject string
	}{
		{"middle.hide", func(c *Client) error { return c.HideMiddleText(context.Background()) }, "tripbot.stage.onscreens.middle.hide"},
		{"timewarp.show", func(c *Client) error { return c.ShowTimewarp(context.Background()) }, "tripbot.stage.onscreens.timewarp.show"},
		{"gps.show", func(c *Client) error { return c.ShowGPSImage(context.Background(), 60) }, "tripbot.stage.onscreens.gps.show"},
		{"gps.hide", func(c *Client) error { return c.HideGPSImage(context.Background()) }, "tripbot.stage.onscreens.gps.hide"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordingPublisher{}
			c := New(host, rec, "stage")
			if err := tc.call(c); err != nil {
				t.Fatalf("call: %v", err)
			}
			if len(rec.Publishes) != 1 {
				t.Fatalf("expected 1 publish, got %d", len(rec.Publishes))
			}
			if rec.Publishes[0].Subject != tc.subject {
				t.Errorf("subject = %q, want %q", rec.Publishes[0].Subject, tc.subject)
			}
			var env oe.Command
			if err := json.Unmarshal(rec.Publishes[0].Payload, &env); err != nil {
				t.Fatalf("payload not valid JSON: %v", err)
			}
			if env.EmittedAt == "" {
				t.Error("emitted_at empty")
			}
		})
	}
}

// TestShowFlagPublishes asserts flag.show publishes a FlagShow carrying the
// state normalized to its two-letter abbrev.
func TestShowFlagPublishes(t *testing.T) {
	rec := &recordingPublisher{}
	c := New(okServer(t), rec, "stage")
	if err := c.ShowFlag(context.Background(), "Missouri", 10); err != nil {
		t.Fatalf("ShowFlag: %v", err)
	}
	if len(rec.Publishes) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(rec.Publishes))
	}
	pub := rec.Publishes[0]
	if pub.Subject != "tripbot.stage.onscreens.flag.show" {
		t.Errorf("subject = %q, want tripbot.stage.onscreens.flag.show", pub.Subject)
	}
	var ev oe.FlagShow
	if err := json.Unmarshal(pub.Payload, &ev); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if ev.State != "MO" {
		t.Errorf("state = %q, want MO", ev.State)
	}
}

// TestShowFlagUnknownStateNoPublish asserts a state with no known abbrev is a
// no-op (nothing to show).
func TestShowFlagUnknownStateNoPublish(t *testing.T) {
	rec := &recordingPublisher{}
	c := New(okServer(t), rec, "stage")
	if err := c.ShowFlag(context.Background(), "Atlantis", 10); err != nil {
		t.Fatalf("ShowFlag: %v", err)
	}
	if len(rec.Publishes) != 0 {
		t.Errorf("expected 0 publishes for unknown state, got %d", len(rec.Publishes))
	}
}

// TestNilPublisher_NoPublishNoPanic asserts a nil publisher disables the
// mirror without panicking (the path used by HTTP-only test rigs).
func TestNilPublisher_NoPublishNoPanic(t *testing.T) {
	c := New(okServer(t), nil, "test")
	if err := c.ShowMiddleText(context.Background(), "x"); err != nil {
		t.Fatalf("ShowMiddleText: %v", err)
	}
}
