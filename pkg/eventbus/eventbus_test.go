package eventbus

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// recordingPublisher captures every publish so tests can assert on the
// subject + payload. Goroutine-safe so concurrent emits don't race the slice.
// Mirrors recordingNATS in pkg/chatbot.
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

// withRecorder installs a recordingPublisher for the duration of fn and
// restores realPublisher afterward.
func withRecorder(t *testing.T) *recordingPublisher {
	t.Helper()
	rec := &recordingPublisher{}
	SetPublisher(rec)
	t.Cleanup(func() { SetPublisher(realPublisher{}) })
	return rec
}

func TestChatMessageSubject(t *testing.T) {
	for _, env := range []string{"prod", "stage", "development"} {
		if got, want := ChatMessageSubject(env), "tripbot."+env+".chat.message"; got != want {
			t.Errorf("ChatMessageSubject(%q) = %q, want %q", env, got, want)
		}
	}
}

func TestEmitChatMessage(t *testing.T) {
	rec := withRecorder(t)

	// Pin emitted_at so the envelope is deterministic.
	fixed := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	nowFn = func() time.Time { return fixed }
	t.Cleanup(func() { nowFn = func() time.Time { return time.Now().UTC() } })

	EmitChatMessage(context.Background(), "development", "DanaLol", "Hello, World!")

	if len(rec.Publishes) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(rec.Publishes))
	}
	pub := rec.Publishes[0]
	if pub.Subject != "tripbot.development.chat.message" {
		t.Errorf("subject = %q, want tripbot.development.chat.message", pub.Subject)
	}

	var ev ChatMessage
	if err := json.Unmarshal(pub.Payload, &ev); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if ev.Username != "DanaLol" {
		t.Errorf("username = %q, want DanaLol (original case preserved)", ev.Username)
	}
	if ev.Text != "Hello, World!" {
		t.Errorf("text = %q, want %q", ev.Text, "Hello, World!")
	}
	if ev.EmittedAt != fixed.Format(time.RFC3339Nano) {
		t.Errorf("emitted_at = %q, want %q", ev.EmittedAt, fixed.Format(time.RFC3339Nano))
	}
}

// TestEmit_NoNATS_NoPanic asserts the production publisher is a silent no-op
// when NATS is unconfigured (natsclient.Conn() is nil), so local dev / tests
// that never call natsclient.Connect don't crash.
func TestEmit_NoNATS_NoPanic(t *testing.T) {
	SetPublisher(realPublisher{})
	t.Cleanup(func() { SetPublisher(realPublisher{}) })
	// natsclient.Conn() is nil here (Connect never called) — must not panic.
	EmitChatMessage(context.Background(), "test", "u", "x")
}
