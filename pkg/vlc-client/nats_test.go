package vlcClient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	ve "github.com/adanalife/tripbot/pkg/vlc-events"
)

// commandHost is handed to New for the command tests. The four commands are
// NATS-only (no HTTP after the peel), so this host is never dialed — it just
// satisfies New's signature.
const commandHost = "vlc.invalid:0"

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

// okServer stands up an httptest.Server that 200s everything, for the one
// remaining HTTP path — CurrentlyPlaying (a read).
func okServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

func TestPlayRandom_PublishesToNATS(t *testing.T) {
	rec := &recordingPublisher{}
	c := New(commandHost, rec, "stage")

	if err := c.PlayRandom(context.Background()); err != nil {
		t.Fatalf("PlayRandom: %v", err)
	}

	if len(rec.Publishes) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(rec.Publishes))
	}
	if rec.Publishes[0].Subject != "tripbot.stage.vlc.play.random" {
		t.Errorf("subject = %q, want tripbot.stage.vlc.play.random", rec.Publishes[0].Subject)
	}
	var ev ve.Command
	if err := json.Unmarshal(rec.Publishes[0].Payload, &ev); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if ev.EmittedAt == "" {
		t.Error("emitted_at empty")
	}
}

func TestPlayFileInPlaylist_PublishesFile(t *testing.T) {
	rec := &recordingPublisher{}
	c := New(commandHost, rec, "prod")

	if err := c.PlayFileInPlaylist(context.Background(), "clip.mp4"); err != nil {
		t.Fatalf("PlayFileInPlaylist: %v", err)
	}

	if len(rec.Publishes) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(rec.Publishes))
	}
	pub := rec.Publishes[0]
	if pub.Subject != "tripbot.prod.vlc.play.file" {
		t.Errorf("subject = %q, want tripbot.prod.vlc.play.file", pub.Subject)
	}
	var ev ve.PlayFile
	if err := json.Unmarshal(pub.Payload, &ev); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if ev.File != "clip.mp4" {
		t.Errorf("file = %q, want clip.mp4", ev.File)
	}
}

func TestSkipAndBack_PublishN(t *testing.T) {
	cases := []struct {
		name    string
		call    func(c *Client) error
		subject string
	}{
		{"skip", func(c *Client) error { return c.Skip(context.Background(), 3) }, "tripbot.stage.vlc.skip"},
		{"back", func(c *Client) error { return c.Back(context.Background(), 2) }, "tripbot.stage.vlc.back"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordingPublisher{}
			c := New(commandHost, rec, "stage")
			if err := tc.call(c); err != nil {
				t.Fatalf("call: %v", err)
			}
			if len(rec.Publishes) != 1 {
				t.Fatalf("expected 1 publish, got %d", len(rec.Publishes))
			}
			if rec.Publishes[0].Subject != tc.subject {
				t.Errorf("subject = %q, want %q", rec.Publishes[0].Subject, tc.subject)
			}
			// Skip and Back share the {n} wire shape — decode as Skip either way.
			var ev ve.Skip
			if err := json.Unmarshal(rec.Publishes[0].Payload, &ev); err != nil {
				t.Fatalf("payload not valid JSON: %v", err)
			}
			want := 3
			if tc.name == "back" {
				want = 2
			}
			if ev.N != want {
				t.Errorf("n = %d, want %d", ev.N, want)
			}
		})
	}
}

// TestCurrentlyPlaying_DoesNotPublish asserts the read stays HTTP-only — it's
// not a fire-and-forget command and must not hit NATS.
func TestCurrentlyPlaying_DoesNotPublish(t *testing.T) {
	rec := &recordingPublisher{}
	c := New(okServer(t), rec, "stage")
	_ = c.CurrentlyPlaying(context.Background())
	if len(rec.Publishes) != 0 {
		t.Errorf("expected 0 publishes for CurrentlyPlaying, got %d", len(rec.Publishes))
	}
}

// TestTopicReflectsEnv covers subject scoping per env.
func TestTopicReflectsEnv(t *testing.T) {
	for _, env := range []string{"prod", "development", "test"} {
		t.Run(env, func(t *testing.T) {
			rec := &recordingPublisher{}
			c := New(commandHost, rec, env)
			if err := c.Skip(context.Background(), 1); err != nil {
				t.Fatalf("Skip: %v", err)
			}
			want := "tripbot." + env + ".vlc.skip"
			if rec.Publishes[0].Subject != want {
				t.Errorf("subject = %q, want %q", rec.Publishes[0].Subject, want)
			}
		})
	}
}

// TestNilPublisher_NoPublishNoPanic asserts a nil publisher disables
// publishing without panicking.
func TestNilPublisher_NoPublishNoPanic(t *testing.T) {
	c := New(commandHost, nil, "test")
	if err := c.Skip(context.Background(), 1); err != nil {
		t.Fatalf("Skip: %v", err)
	}
}
