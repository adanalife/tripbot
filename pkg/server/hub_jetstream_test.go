package server

import (
	"context"
	"testing"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/eventbus"
	"github.com/adanalife/tripbot/pkg/natsclient"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// TestHub_Start_replaysHistoryFromJetStream proves the live console survives
// a reboot. We publish chat + video history,
// then start a *fresh* hub with no SSE clients connected and assert the durable
// JetStream consumers replayed that history into the chat ring and map trail.
// This is delivery-on-startup (replay), not live delivery — the messages were
// published before the hub existed.
func TestHub_Start_replaysHistoryFromJetStream(t *testing.T) {
	// Start spawns pollAuth, which calls the token seam once synchronously. Stub
	// it so the test doesn't reach Twitch/Postgres.
	savedTok := tokenStatusesFn
	tokenStatusesFn = func() []mytwitch.AccountTokenStatus { return nil }
	t.Cleanup(func() { tokenStatusesFn = savedTok })

	nc := connectEmbeddedJetStream(t)
	natsclient.SetConn(nc)
	t.Cleanup(func() { natsclient.SetConn(nil) })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	env := c.Conf.Environment
	js := natsclient.JetStream()
	if js == nil {
		t.Fatal("JetStream() returned nil against a JetStream-enabled server")
	}
	if err := eventbus.EnsureStreams(ctx, js, env); err != nil {
		t.Fatalf("ensure streams: %v", err)
	}

	// Publish history BEFORE the hub starts — the whole point is replay.
	eventbus.EmitChatMessage(ctx, env, "twitch", "alice", "first")
	eventbus.EmitChatMessage(ctx, env, "twitch", "bob", "second")
	eventbus.EmitVideoChanged(ctx, env, "wy_0001.MP4", "Wyoming", false, 41.5, -110.2)
	eventbus.EmitVideoChanged(ctx, env, "ut_0002.MP4", "Utah", false, 40.0, -111.0)
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	waitStreamMsgs(t, js, eventbus.ChatStreamName(), 2)
	waitStreamMsgs(t, js, eventbus.VideoStreamName(), 2)

	// Fresh hub, no clients registered: replay must populate the buffers on its
	// own (broadcast is a no-op with zero subscribers).
	h := NewHub()
	h.Start(ctx)

	waitLen(t, "chat ring", func() int { return len(h.snapshotChat()) }, 2)
	waitLen(t, "map trail", func() int { return len(h.snapshotMapTrail()) }, 2)

	chat := h.snapshotChat()
	if chat[0].Username != "alice" || chat[1].Username != "bob" {
		t.Errorf("chat replay order = %q,%q, want alice,bob", chat[0].Username, chat[1].Username)
	}
	trail := h.snapshotMapTrail()
	if trail[0].Lat != 41.5 || trail[1].Lat != 40.0 {
		t.Errorf("map trail = %+v, want lat 41.5 then 40.0", trail)
	}
}

// connectEmbeddedJetStream starts an in-process JetStream-enabled nats-server on
// a random port with a temp store dir and returns a client connection. Server +
// connection are torn down via t.Cleanup.
func connectEmbeddedJetStream(t *testing.T) *nats.Conn {
	t.Helper()
	ns, err := natsserver.NewServer(&natsserver.Options{
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

// waitStreamMsgs polls until stream holds at least want messages (publishes are
// captured asynchronously by JetStream) or fails after a short deadline.
func waitStreamMsgs(t *testing.T, js jetstream.JetStream, stream string, want uint64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s, err := js.Stream(context.Background(), stream); err == nil {
			if info, err := s.Info(context.Background()); err == nil && info.State.Msgs >= want {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("stream %s did not reach %d msgs in time", stream, want)
}

// waitLen polls get() until it reaches want or fails after a short deadline.
// Used to wait out the async JetStream replay into the hub's buffers.
func waitLen(t *testing.T, what string, get func() int, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if get() >= want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s did not reach len %d (got %d)", what, want, get())
}
