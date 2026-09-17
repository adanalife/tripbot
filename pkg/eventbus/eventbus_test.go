package eventbus

import (
	"bytes"
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

	EmitChatMessage(context.Background(), "development", ChatMessage{
		Platform: "twitch", Username: "DanaLol", UserID: "42",
		Text: "Hello, World!", MessageID: "msg-1", Subscriber: true,
		Badges: map[string]int{"subscriber": 12},
		Emotes: []Emote{{ID: "25", Start: 0, End: 4}},
	})

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
	if ev.UserID != "42" || ev.MessageID != "msg-1" || !ev.Subscriber {
		t.Errorf("identity fields = {user_id:%q message_id:%q subscriber:%v}, want {42 msg-1 true}",
			ev.UserID, ev.MessageID, ev.Subscriber)
	}
	// The decorations survive the round trip with their versions and spans
	// intact — the months a role bool can't carry, and where in the text to
	// draw the emote.
	if ev.Badges["subscriber"] != 12 {
		t.Errorf("badges = %v, want the subscriber badge at version 12", ev.Badges)
	}
	if len(ev.Emotes) != 1 || ev.Emotes[0] != (Emote{ID: "25", Start: 0, End: 4}) {
		t.Errorf("emotes = %+v, want one occurrence of 25 at 0-4", ev.Emotes)
	}
	// Unset roles are omitted rather than published as false, so a consumer
	// can't read "not a moderator" as an answer from a platform that never
	// reports one.
	if bytes.Contains(pub.Payload, []byte("moderator")) {
		t.Errorf("payload carries an unset role: %s", pub.Payload)
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
	EmitChatMessage(context.Background(), "test", ChatMessage{Platform: "twitch", Username: "u", Text: "x"})
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
		{Account: "bot", LoginAs: "tripbot4001", ExpiresAt: fixed.Add(2 * time.Hour).Format(time.RFC3339Nano)},
		{Account: "broadcaster", LoginAs: "adanalife_staging", Reason: "expired"},
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
	if ev.Accounts[1].Reason != "expired" || ev.Accounts[1].LoginAs != "adanalife_staging" {
		t.Errorf("accounts[1] = %+v, want expired broadcaster row", ev.Accounts[1])
	}
	if ev.EmittedAt != fixed.Format(time.RFC3339Nano) {
		t.Errorf("emitted_at = %q, want %q", ev.EmittedAt, fixed.Format(time.RFC3339Nano))
	}
}

func TestYoutubeBroadcastSubject(t *testing.T) {
	for _, env := range []string{"prod", "stage", "development"} {
		if got, want := YoutubeBroadcastSubject(env), "tripbot."+env+".youtube.broadcast"; got != want {
			t.Errorf("YoutubeBroadcastSubject(%q) = %q, want %q", env, got, want)
		}
	}
}

func TestEmitYoutubeBroadcast(t *testing.T) {
	rec := withRecorder(t)

	fixed := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	nowFn = func() time.Time { return fixed }
	t.Cleanup(func() { nowFn = func() time.Time { return time.Now().UTC() } })

	EmitYoutubeBroadcast(context.Background(), "prod", "ka57c6_Jz_o", "unlisted", true)

	if len(rec.Publishes) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(rec.Publishes))
	}
	pub := rec.Publishes[0]
	if pub.Subject != "tripbot.prod.youtube.broadcast" {
		t.Errorf("subject = %q, want tripbot.prod.youtube.broadcast", pub.Subject)
	}

	var ev YoutubeBroadcast
	if err := json.Unmarshal(pub.Payload, &ev); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if ev.VideoID != "ka57c6_Jz_o" || !ev.Live || ev.Privacy != "unlisted" {
		t.Errorf("envelope = %+v, want video_id=ka57c6_Jz_o live=true privacy=unlisted", ev)
	}
	if ev.EmittedAt != fixed.Format(time.RFC3339Nano) {
		t.Errorf("emitted_at = %q, want %q", ev.EmittedAt, fixed.Format(time.RFC3339Nano))
	}
}

func TestFacebookBroadcastSubject(t *testing.T) {
	for _, env := range []string{"prod", "stage", "development"} {
		if got, want := FacebookBroadcastSubject(env), "tripbot."+env+".facebook.broadcast"; got != want {
			t.Errorf("FacebookBroadcastSubject(%q) = %q, want %q", env, got, want)
		}
	}
}

func TestEmitFacebookBroadcast(t *testing.T) {
	rec := withRecorder(t)

	fixed := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	nowFn = func() time.Time { return fixed }
	t.Cleanup(func() { nowFn = func() time.Time { return time.Now().UTC() } })

	EmitFacebookBroadcast(context.Background(), "staging", "10102938475", "1719603579", "/page/videos/10102938475", "unpublished", true)

	if len(rec.Publishes) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(rec.Publishes))
	}
	pub := rec.Publishes[0]
	if pub.Subject != "tripbot.staging.facebook.broadcast" {
		t.Errorf("subject = %q, want tripbot.staging.facebook.broadcast", pub.Subject)
	}

	var ev FacebookBroadcast
	if err := json.Unmarshal(pub.Payload, &ev); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if ev.VideoID != "10102938475" || !ev.Live || ev.Privacy != "unpublished" {
		t.Errorf("envelope = %+v, want video_id=10102938475 live=true privacy=unpublished", ev)
	}
	if ev.BroadcastID != "1719603579" || ev.PermalinkURL != "/page/videos/10102938475" {
		t.Errorf("envelope = %+v, want broadcast_id=1719603579 permalink_url=/page/videos/10102938475", ev)
	}
}

func TestSubscriberEventSubject(t *testing.T) {
	for _, env := range []string{"prod", "stage", "development"} {
		if got, want := SubscriberEventSubject(env), "tripbot."+env+".chat.subscriber"; got != want {
			t.Errorf("SubscriberEventSubject(%q) = %q, want %q", env, got, want)
		}
	}
}

// TestEmitSubscriberEvent walks the four kinds, asserting each carries the
// fields its treatment needs and omits the ones it doesn't — a consumer
// switching on kind must not see a zero tier or month count it might render.
func TestEmitSubscriberEvent(t *testing.T) {
	fixed := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		ev      SubscriberEvent
		want    SubscriberEvent
		absent  []string
		present []string
	}{
		{
			name:   "follow carries only the username",
			ev:     SubscriberEvent{Platform: "twitch", Kind: "follow", Username: "DanaLol"},
			want:   SubscriberEvent{Platform: "twitch", Kind: "follow", Username: "DanaLol"},
			absent: []string{"tier", "months", "streak", "gift_count", "message"},
		},
		{
			name:    "sub carries the tier",
			ev:      SubscriberEvent{Platform: "twitch", Kind: "sub", Username: "DanaLol", Tier: "1000"},
			want:    SubscriberEvent{Platform: "twitch", Kind: "sub", Username: "DanaLol", Tier: "1000"},
			absent:  []string{"months", "streak", "gift_count", "message"},
			present: []string{"tier"},
		},
		{
			name: "anonymous gift carries an empty gifter and the count",
			ev: SubscriberEvent{
				Platform: "twitch", Kind: "gift", Tier: "1000",
				GiftCount: 5, IsAnonymous: true,
			},
			want: SubscriberEvent{
				Platform: "twitch", Kind: "gift", Tier: "1000",
				GiftCount: 5, IsAnonymous: true,
			},
			// username is not omitempty, so the empty gifter is published
			// explicitly rather than dropped — the console needs to tell an
			// anonymous gift from a missing field.
			present: []string{"username", "gift_count", "is_anonymous"},
			absent:  []string{"months", "streak", "message"},
		},
		{
			name: "resub carries cumulative months, streak and message",
			ev: SubscriberEvent{
				Platform: "twitch", Kind: "resub", Username: "DanaLol", Tier: "2000",
				Months: 24, Streak: 6, Message: "still here!",
			},
			want: SubscriberEvent{
				Platform: "twitch", Kind: "resub", Username: "DanaLol", Tier: "2000",
				Months: 24, Streak: 6, Message: "still here!",
			},
			present: []string{"months", "streak", "message"},
			absent:  []string{"gift_count", "is_anonymous"},
		},
		{
			name: "a hidden streak is omitted rather than published as zero",
			ev: SubscriberEvent{
				Platform: "twitch", Kind: "resub", Username: "DanaLol", Tier: "1000",
				Months: 3, Streak: 0,
			},
			want: SubscriberEvent{
				Platform: "twitch", Kind: "resub", Username: "DanaLol", Tier: "1000",
				Months: 3,
			},
			absent: []string{"streak"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := withRecorder(t)
			nowFn = func() time.Time { return fixed }
			t.Cleanup(func() { nowFn = func() time.Time { return time.Now().UTC() } })

			EmitSubscriberEvent(context.Background(), "development", tt.ev)

			if len(rec.Publishes) != 1 {
				t.Fatalf("expected 1 publish, got %d", len(rec.Publishes))
			}
			pub := rec.Publishes[0]
			if pub.Subject != "tripbot.development.chat.subscriber" {
				t.Errorf("subject = %q, want tripbot.development.chat.subscriber", pub.Subject)
			}

			var got SubscriberEvent
			if err := json.Unmarshal(pub.Payload, &got); err != nil {
				t.Fatalf("payload not valid JSON: %v", err)
			}
			if got.EmittedAt != fixed.Format(time.RFC3339Nano) {
				t.Errorf("emitted_at = %q, want %q", got.EmittedAt, fixed.Format(time.RFC3339Nano))
			}
			// EmittedAt is stamped by the emit, not the caller.
			want := tt.want
			want.EmittedAt = got.EmittedAt
			if got != want {
				t.Errorf("envelope = %+v, want %+v", got, want)
			}
			for _, key := range tt.absent {
				if bytes.Contains(pub.Payload, []byte(`"`+key+`"`)) {
					t.Errorf("payload carries an unset %s: %s", key, pub.Payload)
				}
			}
			for _, key := range tt.present {
				if !bytes.Contains(pub.Payload, []byte(`"`+key+`"`)) {
					t.Errorf("payload is missing %s: %s", key, pub.Payload)
				}
			}
		})
	}
}
