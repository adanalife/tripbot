package onscreensServer

import (
	"context"
	"testing"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/onscreens-server"
	"github.com/adanalife/tripbot/pkg/natsclient"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// TestMiddleStateRoundTrip proves the restore-on-restart store end to end
// against an embedded JetStream server: the last published state wins
// (last-value cache), and both content and visibility round-trip.
func TestMiddleStateRoundTrip(t *testing.T) {
	nc := connectEmbeddedJetStream(t)
	natsclient.SetConn(nc)
	t.Cleanup(func() { natsclient.SetConn(nil) })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	const env = "testing"
	js := natsclient.JetStream()
	if js == nil {
		t.Fatal("JetStream() returned nil against a JetStream-enabled server")
	}
	if err := EnsureMiddleStateStream(ctx, js, env); err != nil {
		t.Fatalf("ensure stream: %v", err)
	}

	publishMiddleState(ctx, env, "first text", true)
	publishMiddleState(ctx, env, "second text", true) // supersedes first
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// JetStream captures core publishes asynchronously; poll until the cache
	// shows the superseding value.
	waitMiddleState(t, ctx, js, env, "second text", true)

	// A hide retains the content but flips visibility.
	publishMiddleState(ctx, env, "second text", false)
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	waitMiddleState(t, ctx, js, env, "second text", false)
}

// TestRestoreMiddleText proves Server.RestoreMiddleText overrides the
// overlay's constructed default with the persisted state.
func TestRestoreMiddleText(t *testing.T) {
	nc := connectEmbeddedJetStream(t)
	natsclient.SetConn(nc)
	t.Cleanup(func() { natsclient.SetConn(nil) })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	// RestoreMiddleText reads under c.Conf.Environment ("testing" in tests);
	// ensure + publish under the same env so the restore finds the state.
	js := natsclient.JetStream()
	env := c.Conf.Environment
	if err := EnsureMiddleStateStream(ctx, js, env); err != nil {
		t.Fatalf("ensure stream: %v", err)
	}
	publishMiddleState(ctx, env, "restored text", true)
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	waitMiddleState(t, ctx, js, env, "restored text", true)

	s := &Server{MiddleText: newMiddleText()}
	s.RestoreMiddleText(ctx)

	if s.MiddleText.Content != "restored text" {
		t.Errorf("MiddleText.Content = %q, want %q", s.MiddleText.Content, "restored text")
	}
	if !s.MiddleText.IsShowing {
		t.Errorf("MiddleText.IsShowing = false, want true")
	}
}

// TestMiddleStateNilJetStream proves the paths degrade to "nothing to
// restore" when NATS is unconfigured.
func TestMiddleStateNilJetStream(t *testing.T) {
	if _, _, ok := readMiddleState(context.Background(), nil, "testing"); ok {
		t.Errorf("readMiddleState with nil js = ok=true, want false")
	}
	if err := EnsureMiddleStateStream(context.Background(), nil, "testing"); err != nil {
		t.Errorf("EnsureMiddleStateStream with nil js = %v, want nil", err)
	}
}

// waitMiddleState polls the last-value cache until it reads wantMsg/wantShowing,
// or fails after a short deadline.
func waitMiddleState(t *testing.T, ctx context.Context, js jetstream.JetStream, env, wantMsg string, wantShowing bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var gotMsg string
	var gotShowing, ok bool
	for time.Now().Before(deadline) {
		if gotMsg, gotShowing, ok = readMiddleState(ctx, js, env); ok && gotMsg == wantMsg && gotShowing == wantShowing {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("middle state = %q/showing=%v (ok=%v), want %q/showing=%v", gotMsg, gotShowing, ok, wantMsg, wantShowing)
}

// connectEmbeddedJetStream starts an in-process JetStream-enabled nats-server
// on a random port with a temp store dir and returns a client connection.
// (Same fixture shape as pkg/vlc-server's lastplayed_test.)
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
