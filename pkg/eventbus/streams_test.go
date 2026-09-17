package eventbus

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/natsclient"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// connectEmbeddedNATS starts an in-process JetStream-enabled nats-server on an
// ephemeral port with a temp store dir and returns a client connection. Same
// fixture shape as pkg/onscreens-server's middle-state tests.
func connectEmbeddedNATS(t *testing.T) *nats.Conn {
	t.Helper()
	ns, err := server.NewServer(&server.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("new nats server: %v", err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(10 * time.Second) {
		t.Fatal("embedded nats server not ready")
	}
	t.Cleanup(ns.Shutdown)

	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatalf("connect to embedded server: %v", err)
	}
	t.Cleanup(nc.Close)
	return nc
}

// connectEmbeddedJetStream returns both a client connection to a fresh embedded
// server and a JetStream context on it.
func connectEmbeddedJetStream(t *testing.T) (*nats.Conn, jetstream.JetStream) {
	t.Helper()
	nc := connectEmbeddedNATS(t)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("jetstream context: %v", err)
	}
	return nc, js
}

// wantStream is the config EnsureStreams is expected to leave behind. An unset
// limit reads back as -1 from the server, which is what distinguishes a
// bounded ring (chat, video) from a last-value cache (auth, youtube, facebook).
type wantStream struct {
	subjects          []string
	maxMsgs           int64
	maxMsgsPerSubject int64
}

func expectedStreams(env string) map[string]wantStream {
	return map[string]wantStream{
		chatStreamName: {
			subjects: []string{ChatMessageSubject(env)}, maxMsgs: chatStreamMaxMsgs, maxMsgsPerSubject: -1,
		},
		videoStreamName: {
			subjects: []string{VideoChangedSubject(env)}, maxMsgs: videoStreamMaxMsgs, maxMsgsPerSubject: -1,
		},
		authStreamName: {
			subjects: []string{AuthStatusWildcard(env)}, maxMsgs: -1, maxMsgsPerSubject: 1,
		},
		youtubeStreamName: {
			subjects: []string{YoutubeBroadcastSubject(env)}, maxMsgs: -1, maxMsgsPerSubject: 1,
		},
		facebookStreamName: {
			subjects: []string{FacebookBroadcastSubject(env)}, maxMsgs: -1, maxMsgsPerSubject: 1,
		},
	}
}

func checkStreams(t *testing.T, ctx context.Context, js jetstream.JetStream, env string) {
	t.Helper()
	for name, want := range expectedStreams(env) {
		s, err := js.Stream(ctx, name)
		if err != nil {
			t.Errorf("stream %s: %v", name, err)
			continue
		}
		cfg := s.CachedInfo().Config
		if !slices.Equal(cfg.Subjects, want.subjects) {
			t.Errorf("%s subjects = %v, want %v", name, cfg.Subjects, want.subjects)
		}
		if cfg.MaxMsgs != want.maxMsgs {
			t.Errorf("%s MaxMsgs = %d, want %d", name, cfg.MaxMsgs, want.maxMsgs)
		}
		if cfg.MaxMsgsPerSubject != want.maxMsgsPerSubject {
			t.Errorf("%s MaxMsgsPerSubject = %d, want %d", name, cfg.MaxMsgsPerSubject, want.maxMsgsPerSubject)
		}
		if cfg.Storage != jetstream.FileStorage {
			t.Errorf("%s Storage = %v, want file (history must survive a reboot)", name, cfg.Storage)
		}
		if cfg.Retention != jetstream.LimitsPolicy {
			t.Errorf("%s Retention = %v, want limits", name, cfg.Retention)
		}
		if cfg.Discard != jetstream.DiscardOld {
			t.Errorf("%s Discard = %v, want old (a full stream evicts, never rejects)", name, cfg.Discard)
		}
		if cfg.Description == "" {
			t.Errorf("%s has no description", name)
		}
	}
}

func TestEnsureStreams(t *testing.T) {
	ctx := context.Background()
	_, js := connectEmbeddedJetStream(t)

	if err := EnsureStreams(ctx, js, "development"); err != nil {
		t.Fatalf("EnsureStreams: %v", err)
	}
	checkStreams(t, ctx, js, "development")
}

// TestEnsureStreams_Idempotent asserts a second call on an already-declared
// JetStream succeeds and leaves the config alone — it runs on every startup.
func TestEnsureStreams_Idempotent(t *testing.T) {
	ctx := context.Background()
	_, js := connectEmbeddedJetStream(t)

	for i := 1; i <= 2; i++ {
		if err := EnsureStreams(ctx, js, "development"); err != nil {
			t.Fatalf("EnsureStreams call %d: %v", i, err)
		}
	}
	checkStreams(t, ctx, js, "development")

	var count int
	for range js.StreamNames(ctx).Name() {
		count++
	}
	if count != len(expectedStreams("development")) {
		t.Errorf("stream count = %d, want %d (a repeat call must not fork duplicates)", count, len(expectedStreams("development")))
	}
}

// TestEnsureStreams_ReconcilesDrift asserts a stream that already exists with a
// stale cap is updated in place rather than erroring on the conflict — how a
// retention change ships.
func TestEnsureStreams_ReconcilesDrift(t *testing.T) {
	ctx := context.Background()
	_, js := connectEmbeddedJetStream(t)

	if _, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:      chatStreamName,
		Subjects:  []string{ChatMessageSubject("development")},
		Storage:   jetstream.FileStorage,
		Retention: jetstream.LimitsPolicy,
		Discard:   jetstream.DiscardOld,
		MaxMsgs:   7,
	}); err != nil {
		t.Fatalf("seed drifted stream: %v", err)
	}

	if err := EnsureStreams(ctx, js, "development"); err != nil {
		t.Fatalf("EnsureStreams over drifted config: %v", err)
	}
	checkStreams(t, ctx, js, "development")
}

// TestEnsureStreams_NilJetStream asserts the no-op path: NATS off or JetStream
// unavailable leaves the hub on live-only core subscriptions rather than
// failing startup.
func TestEnsureStreams_NilJetStream(t *testing.T) {
	if err := EnsureStreams(context.Background(), nil, "development"); err != nil {
		t.Errorf("EnsureStreams(nil) = %v, want nil", err)
	}
}

// TestEnsureStreams_CapturesCorePublish asserts the declared subjects match what
// the Emit helpers actually publish: a plain core publish lands in the stream,
// which is what lets the console backfill after a restart.
func TestEnsureStreams_CapturesCorePublish(t *testing.T) {
	ctx := context.Background()
	nc, js := connectEmbeddedJetStream(t)

	if err := EnsureStreams(ctx, js, "development"); err != nil {
		t.Fatalf("EnsureStreams: %v", err)
	}

	natsclient.SetConn(nc)
	t.Cleanup(func() { natsclient.SetConn(nil) })
	SetPublisher(realPublisher{})
	t.Cleanup(func() { SetPublisher(realPublisher{}) })

	EmitChatMessage(ctx, "development", ChatMessage{Platform: "twitch", Username: "DanaLol", Text: "backfill me"})
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	s, err := js.Stream(ctx, chatStreamName)
	if err != nil {
		t.Fatalf("stream %s: %v", chatStreamName, err)
	}
	msg, err := s.GetMsg(ctx, 1)
	if err != nil {
		t.Fatalf("get first stored message: %v", err)
	}
	if msg.Subject != ChatMessageSubject("development") {
		t.Errorf("stored subject = %q, want %q", msg.Subject, ChatMessageSubject("development"))
	}
	var ev ChatMessage
	if err := json.Unmarshal(msg.Data, &ev); err != nil {
		t.Fatalf("stored payload not valid JSON: %v", err)
	}
	if ev.Text != "backfill me" || ev.Username != "DanaLol" {
		t.Errorf("stored envelope = %+v, want the emitted chat line", ev)
	}
}

// TestEmitSubscriberEvent_RoundTrip asserts a subscriber on the canonical
// subject receives the envelope the production publisher sends — the console's
// path, end to end over a real server. Not streamed: these are live-only
// moments, so a core subscription is all there is.
func TestEmitSubscriberEvent_RoundTrip(t *testing.T) {
	ctx := context.Background()
	nc := connectEmbeddedNATS(t)

	natsclient.SetConn(nc)
	t.Cleanup(func() { natsclient.SetConn(nil) })
	SetPublisher(realPublisher{})
	t.Cleanup(func() { SetPublisher(realPublisher{}) })

	sub, err := nc.SubscribeSync(SubscriberEventSubject("development"))
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush subscription: %v", err)
	}

	EmitSubscriberEvent(ctx, "development", SubscriberEvent{
		Platform: "twitch", Kind: "resub", Username: "DanaLol", Tier: "2000",
		Months: 24, Streak: 6, Message: "still here!",
	})

	msg, err := sub.NextMsg(5 * time.Second)
	if err != nil {
		t.Fatalf("no subscriber event received: %v", err)
	}
	if msg.Subject != "tripbot.development.chat.subscriber" {
		t.Errorf("subject = %q, want tripbot.development.chat.subscriber", msg.Subject)
	}

	var ev SubscriberEvent
	if err := json.Unmarshal(msg.Data, &ev); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if ev.Kind != "resub" || ev.Username != "DanaLol" || ev.Tier != "2000" {
		t.Errorf("envelope = %+v, want a resub from DanaLol at tier 2000", ev)
	}
	if ev.Months != 24 || ev.Streak != 6 || ev.Message != "still here!" {
		t.Errorf("envelope = %+v, want months=24 streak=6 message=%q", ev, "still here!")
	}
	if _, err := time.Parse(time.RFC3339Nano, ev.EmittedAt); err != nil {
		t.Errorf("emitted_at = %q, not RFC3339Nano: %v", ev.EmittedAt, err)
	}
}

// TestEnsureStreams_SubjectOverlapErrors asserts the error is wrapped with the
// stream that failed. A second stream already claiming one of these subjects is
// how the conflict shows up — JetStream refuses overlapping subjects.
func TestEnsureStreams_SubjectOverlapErrors(t *testing.T) {
	ctx := context.Background()
	_, js := connectEmbeddedJetStream(t)

	if _, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "SQUATTER",
		Subjects: []string{ChatMessageSubject("development")},
		Storage:  jetstream.MemoryStorage,
	}); err != nil {
		t.Fatalf("seed squatting stream: %v", err)
	}

	err := EnsureStreams(ctx, js, "development")
	if err == nil {
		t.Fatal("EnsureStreams over an overlapping subject = nil, want an error")
	}
	if !strings.Contains(err.Error(), "ensure stream "+chatStreamName) {
		t.Errorf("error = %q, want it to name the stream that failed", err)
	}
}

// TestEmit_MarshalFailure asserts an unmarshalable event is dropped rather than
// published as a broken payload a consumer would have to defend against.
func TestEmit_MarshalFailure(t *testing.T) {
	rec := withRecorder(t)

	emit(context.Background(), "tripbot.development.chat.message", func() {})

	if len(rec.Publishes) != 0 {
		t.Errorf("published %d messages, want 0 (marshal failed)", len(rec.Publishes))
	}
}
