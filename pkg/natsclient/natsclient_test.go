package natsclient

import (
	"testing"

	"github.com/nats-io/nats.go"
)

// TestConnectRetriesUnreachableServer pins the boot-race behavior: a dial
// failure must return a live, retrying conn (subscriptions queue and replay
// on connect) rather than nil — nil left the process permanently deaf.
func TestConnectRetriesUnreachableServer(t *testing.T) {
	t.Cleanup(func() { SetConn(nil) })
	SetConn(nil)

	// A port nothing listens on: dial fails, retry loop takes over.
	conn := Connect("nats://127.0.0.1:1", "natsclient-test")
	if conn == nil {
		t.Fatal("Connect returned nil for an unreachable server; want a retrying conn")
	}
	if conn.IsConnected() {
		t.Fatal("conn unexpectedly connected to a dead port")
	}
	// Subscriptions on a not-yet-connected conn must queue, not error.
	if _, err := conn.Subscribe("test.subject", func(*nats.Msg) {}); err != nil {
		t.Fatalf("Subscribe on retrying conn errored: %v", err)
	}
	conn.Close()
}

// TestConnectEmptyURLStaysNil pins the local-dev contract: no URL, no conn.
func TestConnectEmptyURLStaysNil(t *testing.T) {
	t.Cleanup(func() { SetConn(nil) })
	SetConn(nil)

	if conn := Connect("", "natsclient-test"); conn != nil {
		t.Fatal("Connect with empty URL returned a conn; want nil no-op mode")
	}
}
