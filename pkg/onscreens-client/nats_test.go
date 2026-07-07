package onscreensClient

import (
	"context"
	"encoding/json"
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

// TestShowMiddleText_PublishesToNATS asserts the client fires the right
// subject + envelope on every ShowMiddleText.
func TestShowMiddleText_PublishesToNATS(t *testing.T) {
	rec := &recordingPublisher{}
	c := New(rec, "stage", "twitch")

	if err := c.ShowMiddleText(context.Background(), "hello world"); err != nil {
		t.Fatalf("ShowMiddleText: %v", err)
	}

	if len(rec.Publishes) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(rec.Publishes))
	}
	pub := rec.Publishes[0]
	if pub.Subject != "tripbot.stage.onscreens.middle.show.twitch" {
		t.Errorf("subject = %q, want tripbot.stage.onscreens.middle.show.twitch", pub.Subject)
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

// TestShowMiddleText_TopicReflectsEnvAndPlatform covers the subject scoping
// per env AND per streaming platform — the trailing platform leaf is what
// keeps a Twitch overlay off the YouTube stream.
func TestShowMiddleText_TopicReflectsEnvAndPlatform(t *testing.T) {
	cases := []struct{ env, platform string }{
		{"prod", "twitch"},
		{"development", "youtube"},
		{"test", "twitch"},
	}
	for _, tc := range cases {
		t.Run(tc.env+"/"+tc.platform, func(t *testing.T) {
			rec := &recordingPublisher{}
			c := New(rec, tc.env, tc.platform)
			if err := c.ShowMiddleText(context.Background(), "x"); err != nil {
				t.Fatalf("ShowMiddleText: %v", err)
			}
			if len(rec.Publishes) != 1 {
				t.Fatalf("expected 1 publish")
			}
			want := "tripbot." + tc.env + ".onscreens.middle.show." + tc.platform
			if rec.Publishes[0].Subject != want {
				t.Errorf("subject = %q, want %q", rec.Publishes[0].Subject, want)
			}
		})
	}
}

// TestShowLeaderboard_PublishesToNATS asserts the leaderboard publish carries
// the structured {title, rows} payload (the server renders it).
func TestShowLeaderboard_PublishesToNATS(t *testing.T) {
	rec := &recordingPublisher{}
	c := New(rec, "prod", "twitch")

	rows := [][]string{{"alice", "100"}, {"bob", "50"}}
	if err := c.ShowLeaderboard(context.Background(), "Monthly Miles", rows); err != nil {
		t.Fatalf("ShowLeaderboard: %v", err)
	}

	if len(rec.Publishes) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(rec.Publishes))
	}
	pub := rec.Publishes[0]
	if pub.Subject != "tripbot.prod.onscreens.leaderboard.show.twitch" {
		t.Errorf("subject = %q, want tripbot.prod.onscreens.leaderboard.show.twitch", pub.Subject)
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

// TestEmptyPayloadCommandsPublish covers the no-payload commands: each fires
// exactly one publish on its subject with an envelope that carries emitted_at.
func TestEmptyPayloadCommandsPublish(t *testing.T) {
	cases := []struct {
		name    string
		call    func(c *Client) error
		subject string
	}{
		{"middle.hide", func(c *Client) error { return c.HideMiddleText(context.Background()) }, "tripbot.stage.onscreens.middle.hide.twitch"},
		{"timewarp.show", func(c *Client) error { return c.ShowTimewarp(context.Background(), "viewer1") }, "tripbot.stage.onscreens.timewarp.show.twitch"},
		{"gps.show", func(c *Client) error { return c.ShowGPSImage(context.Background(), 60) }, "tripbot.stage.onscreens.gps.show.twitch"},
		{"gps.hide", func(c *Client) error { return c.HideGPSImage(context.Background()) }, "tripbot.stage.onscreens.gps.hide.twitch"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordingPublisher{}
			c := New(rec, "stage", "twitch")
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

// TestNilPublisher_NoPublishNoPanic asserts a nil publisher disables
// publishing without panicking.
func TestNilPublisher_NoPublishNoPanic(t *testing.T) {
	c := New(nil, "test", "twitch")
	if err := c.ShowMiddleText(context.Background(), "x"); err != nil {
		t.Fatalf("ShowMiddleText: %v", err)
	}
}
