package natsclient

import (
	natsserver "github.com/nats-io/nats-server/v2/server"
	"net"
	"strconv"
	"testing"
	"time"

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

// TestConnectCallbackFiresAfterLateServer pins the boot-race contract the
// on-connect callback exists for: a Connect issued before the server listens
// still runs its callbacks once the retry loop gets through, so JetStream
// declares and restores placed there execute against a live server.
func TestConnectCallbackFiresAfterLateServer(t *testing.T) {
	t.Cleanup(func() { SetConn(nil) })
	SetConn(nil)

	// Reserve a port nothing serves yet, then release it for the server.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	connected := make(chan struct{}, 1)
	conn := Connect("nats://127.0.0.1:"+strconv.Itoa(port), "natsclient-test", func(*nats.Conn) {
		connected <- struct{}{}
	})
	if conn == nil {
		t.Fatal("Connect returned nil for a not-yet-listening server; want a retrying conn")
	}
	t.Cleanup(conn.Close)
	if conn.IsConnected() {
		t.Fatal("conn unexpectedly connected before the server started")
	}

	ns, err := natsserver.NewServer(&natsserver.Options{Host: "127.0.0.1", Port: port, NoLog: true, NoSigs: true})
	if err != nil {
		t.Fatal(err)
	}
	go ns.Start()
	t.Cleanup(ns.Shutdown)
	if !ns.ReadyForConnections(5 * time.Second) {
		t.Fatal("embedded nats-server not ready")
	}

	select {
	case <-connected:
	case <-time.After(3 * reconnectWait):
		t.Fatal("on-connect callback never ran after the server came up")
	}
}
