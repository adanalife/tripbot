package chatbot

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
)

// noopNATS satisfies NATS for tests that don't care whether anything
// got published.
type noopNATS struct{}

func (noopNATS) Publish(_ context.Context, _ string, _ []byte) {}

// recordingNATS captures every publish call so tests can assert on the
// subject + payload pair. Goroutine-safe so concurrent commands don't
// race the slice.
type recordingNATS struct {
	mu       sync.Mutex
	Publishes []recordedPublish
}

type recordedPublish struct {
	Subject string
	Payload []byte
}

func (r *recordingNATS) Publish(_ context.Context, subject string, payload []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]byte, len(payload))
	copy(cp, payload)
	r.Publishes = append(r.Publishes, recordedPublish{Subject: subject, Payload: cp})
}

// TestRealOnscreens_ShowMiddleText_PublishesToNATS asserts realOnscreens
// fires the right subject + envelope shape on every ShowMiddleText call,
// regardless of what the HTTP path does.
func TestRealOnscreens_ShowMiddleText_PublishesToNATS(t *testing.T) {
	rec := &recordingNATS{}
	r := realOnscreens{c: nil, nats: rec, env: "stage"}

	// Calling with a nil HTTP client panics — but we want to assert the
	// publish happened *before* the HTTP call. Catch the panic and proceed.
	func() {
		defer func() { _ = recover() }()
		_ = r.ShowMiddleText(context.Background(), "hello world")
	}()

	if len(rec.Publishes) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(rec.Publishes))
	}
	pub := rec.Publishes[0]
	if pub.Subject != "tripbot.stage.onscreens.middle.show" {
		t.Errorf("subject = %q, want tripbot.stage.onscreens.middle.show", pub.Subject)
	}

	var ev middleTextEvent
	if err := json.Unmarshal(pub.Payload, &ev); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if ev.Msg != "hello world" {
		t.Errorf("msg = %q, want hello world", ev.Msg)
	}
	if ev.EmittedAt == "" {
		t.Errorf("emitted_at empty")
	}
}

// TestRealOnscreens_ShowMiddleText_TopicReflectsEnv covers prod, dev,
// any non-stage env wires through correctly.
func TestRealOnscreens_ShowMiddleText_TopicReflectsEnv(t *testing.T) {
	for _, env := range []string{"prod", "development", "test"} {
		t.Run(env, func(t *testing.T) {
			rec := &recordingNATS{}
			r := realOnscreens{c: nil, nats: rec, env: env}
			func() {
				defer func() { _ = recover() }()
				_ = r.ShowMiddleText(context.Background(), "x")
			}()
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
