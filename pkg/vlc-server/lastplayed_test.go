package vlcServer

import (
	"context"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/natsclient"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// TestLastPlayedRoundTrip proves the resume-on-restart store end to end
// against an embedded JetStream server: each platform leaf keeps exactly the
// latest publish (last-value cache), and the leaves don't clobber each other.
func TestLastPlayedRoundTrip(t *testing.T) {
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
	if err := EnsureLastPlayedStream(ctx, js, env); err != nil {
		t.Fatalf("ensure stream: %v", err)
	}

	publishLastPlayed(ctx, env, "twitch", "wy_0001.MP4", 0)
	publishLastPlayed(ctx, env, "youtube", "ut_0002.MP4", 90_000)
	publishLastPlayed(ctx, env, "twitch", "wy_0003.MP4", 42_500) // supersedes wy_0001
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// JetStream captures core publishes asynchronously; poll until the twitch
	// leaf shows the superseding value (file AND position).
	waitLastPlayed(t, ctx, js, env, "twitch", "wy_0003.MP4", 42_500)
	waitLastPlayed(t, ctx, js, env, "youtube", "ut_0002.MP4", 90_000)

	// A platform that never published has nothing to resume.
	if file, _, ok := lastPlayed(ctx, js, env, "kick"); ok {
		t.Errorf("lastPlayed for unpublished platform = %q, want ok=false", file)
	}
}

// TestLastPlayedNilJetStream proves the read path degrades to "nothing to
// resume" when NATS is unconfigured.
func TestLastPlayedNilJetStream(t *testing.T) {
	if file, _, ok := lastPlayed(context.Background(), nil, "testing", "twitch"); ok {
		t.Errorf("lastPlayed with nil js = %q, want ok=false", file)
	}
	if err := EnsureLastPlayedStream(context.Background(), nil, "testing"); err != nil {
		t.Errorf("EnsureLastPlayedStream with nil js = %v, want nil", err)
	}
}

// connectEmbeddedJetStream starts an in-process JetStream-enabled nats-server
// on a random port with a temp store dir and returns a client connection.
// (Same fixture shape as pkg/server's hub_jetstream_test.)
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

// waitLastPlayed polls the last-value cache until the platform leaf reads
// wantFile at wantPos, or fails after a short deadline.
func waitLastPlayed(t *testing.T, ctx context.Context, js jetstream.JetStream, env, platform, wantFile string, wantPos int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var gotFile string
	var gotPos int64
	var ok bool
	for time.Now().Before(deadline) {
		if gotFile, gotPos, ok = lastPlayed(ctx, js, env, platform); ok && gotFile == wantFile && gotPos == wantPos {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("lastplayed %s/%s = %q@%d (ok=%v), want %q@%d", env, platform, gotFile, gotPos, ok, wantFile, wantPos)
}
