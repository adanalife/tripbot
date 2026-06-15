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

	EmitChatMessage(context.Background(), "development", "twitch", "DanaLol", "Hello, World!")

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

func TestViewerCountSubject(t *testing.T) {
	for _, env := range []string{"prod", "stage", "development"} {
		if got, want := ViewerCountSubject(env), "tripbot."+env+".viewers.count"; got != want {
			t.Errorf("ViewerCountSubject(%q) = %q, want %q", env, got, want)
		}
	}
}

func TestEmitViewerCount(t *testing.T) {
	rec := withRecorder(t)

	fixed := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	nowFn = func() time.Time { return fixed }
	t.Cleanup(func() { nowFn = func() time.Time { return time.Now().UTC() } })

	EmitViewerCount(context.Background(), "development", "twitch", 42)

	if len(rec.Publishes) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(rec.Publishes))
	}
	pub := rec.Publishes[0]
	if pub.Subject != "tripbot.development.viewers.count" {
		t.Errorf("subject = %q, want tripbot.development.viewers.count", pub.Subject)
	}

	var ev ViewerCount
	if err := json.Unmarshal(pub.Payload, &ev); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if ev.Platform != "twitch" {
		t.Errorf("platform = %q, want twitch", ev.Platform)
	}
	if ev.Count != 42 {
		t.Errorf("count = %d, want 42", ev.Count)
	}
	if ev.EmittedAt != fixed.Format(time.RFC3339Nano) {
		t.Errorf("emitted_at = %q, want %q", ev.EmittedAt, fixed.Format(time.RFC3339Nano))
	}
}

func TestVideoChangedSubject(t *testing.T) {
	for _, env := range []string{"prod", "stage", "development"} {
		if got, want := VideoChangedSubject(env), "tripbot."+env+".video.changed"; got != want {
			t.Errorf("VideoChangedSubject(%q) = %q, want %q", env, got, want)
		}
	}
}

func TestEmitVideoChanged(t *testing.T) {
	rec := withRecorder(t)

	fixed := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	nowFn = func() time.Time { return fixed }
	t.Cleanup(func() { nowFn = func() time.Time { return time.Now().UTC() } })

	EmitVideoChanged(context.Background(), "development", "youtube", "wy_0042.MP4", "Wyoming", false, 41.5, -110.2)

	if len(rec.Publishes) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(rec.Publishes))
	}
	pub := rec.Publishes[0]
	if pub.Subject != "tripbot.development.video.changed" {
		t.Errorf("subject = %q, want tripbot.development.video.changed", pub.Subject)
	}

	var ev VideoChanged
	if err := json.Unmarshal(pub.Payload, &ev); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if ev.Platform != "youtube" {
		t.Errorf("platform = %q, want youtube", ev.Platform)
	}
	if ev.File != "wy_0042.MP4" || ev.State != "Wyoming" || ev.Flagged {
		t.Errorf("envelope = %+v, want file=wy_0042.MP4 state=Wyoming flagged=false", ev)
	}
	if ev.Lat != 41.5 || ev.Lng != -110.2 {
		t.Errorf("coords = %v,%v want 41.5,-110.2", ev.Lat, ev.Lng)
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
	EmitChatMessage(context.Background(), "test", "twitch", "u", "x")
}

func TestAuthStatusSubject(t *testing.T) {
	for _, env := range []string{"prod", "stage", "development"} {
		for _, platform := range []string{"twitch", "youtube"} {
			got := AuthStatusSubject(env, platform)
			want := "tripbot." + env + ".auth.status." + platform
			if got != want {
				t.Errorf("AuthStatusSubject(%q, %q) = %q, want %q", env, platform, got, want)
			}
		}
	}
}

func TestAuthStatusWildcard(t *testing.T) {
	if got, want := AuthStatusWildcard("development"), "tripbot.development.auth.status.*"; got != want {
		t.Errorf("AuthStatusWildcard(development) = %q, want %q", got, want)
	}
}

func TestEmitAuthStatus(t *testing.T) {
	rec := withRecorder(t)

	fixed := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	nowFn = func() time.Time { return fixed }
	t.Cleanup(func() { nowFn = func() time.Time { return time.Now().UTC() } })

	accounts := []AuthAccount{
		{Account: "bot", LoginAs: "tripbot4001", ExpiresAt: fixed.Add(2 * time.Hour).Format(time.RFC3339Nano), InitURL: "https://tripbot.example/auth/init?account=bot"},
		{Account: "broadcaster", LoginAs: "adanalife_staging", Reason: "expired", InitURL: "https://tripbot.example/auth/init?account=broadcaster"},
	}
	EmitAuthStatus(context.Background(), "development", "twitch", accounts)

	if len(rec.Publishes) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(rec.Publishes))
	}
	pub := rec.Publishes[0]
	if pub.Subject != "tripbot.development.auth.status.twitch" {
		t.Errorf("subject = %q, want tripbot.development.auth.status.twitch", pub.Subject)
	}

	var ev AuthStatus
	if err := json.Unmarshal(pub.Payload, &ev); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if ev.Platform != "twitch" {
		t.Errorf("platform = %q, want twitch", ev.Platform)
	}
	if len(ev.Accounts) != 2 {
		t.Fatalf("accounts = %d, want 2", len(ev.Accounts))
	}
	if ev.Accounts[0].Account != "bot" || ev.Accounts[0].Reason != "" {
		t.Errorf("accounts[0] = %+v, want healthy bot row", ev.Accounts[0])
	}
	if ev.Accounts[1].Reason != "expired" || ev.Accounts[1].InitURL == "" {
		t.Errorf("accounts[1] = %+v, want expired broadcaster row with InitURL", ev.Accounts[1])
	}
	if ev.EmittedAt != fixed.Format(time.RFC3339Nano) {
		t.Errorf("emitted_at = %q, want %q", ev.EmittedAt, fixed.Format(time.RFC3339Nano))
	}
}
