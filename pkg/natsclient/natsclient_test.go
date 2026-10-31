package natsclient

import (
	"context"
	"errors"
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

// TestConnectRedeclaresStreamsAfterWipedRestart pins the reconnect contract:
// the on-connect callbacks run again after a reconnect, so a NATS restart that
// comes back with empty storage gets its streams re-declared instead of
// silently capturing nothing until this process restarts.
func TestConnectRedeclaresStreamsAfterWipedRestart(t *testing.T) {
	t.Cleanup(func() { SetConn(nil) })
	SetConn(nil)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	start := func() *natsserver.Server {
		t.Helper()
		ns, err := natsserver.NewServer(&natsserver.Options{
			Host: "127.0.0.1", Port: port, NoLog: true, NoSigs: true,
			JetStream: true, StoreDir: t.TempDir(), // a fresh dir each start: the wipe
		})
		if err != nil {
			t.Fatal(err)
		}
		go ns.Start()
		if !ns.ReadyForConnections(5 * time.Second) {
			t.Fatal("embedded nats-server not ready")
		}
		return ns
	}
	ns := start()

	ctx := context.Background()
	declared := make(chan struct{}, 4)
	conn := Connect("nats://127.0.0.1:"+strconv.Itoa(port), "natsclient-test", func(*nats.Conn) {
		if err := RetryDeclare(ctx, func(ctx context.Context) error {
			return EnsureLastValueStream(ctx, JetStream(), "TEST_LV", "test", []string{"test.lv.>"})
		}); err != nil {
			t.Errorf("declare: %v", err)
		}
		declared <- struct{}{}
	})
	if conn == nil {
		t.Fatal("Connect returned nil")
	}
	t.Cleanup(conn.Close)

	waitDeclared := func(when string) {
		t.Helper()
		select {
		case <-declared:
		case <-time.After(10 * reconnectWait):
			t.Fatalf("stream declare never ran %s", when)
		}
	}
	waitDeclared("on first connect")

	ns.Shutdown()
	ns.WaitForShutdown()
	ns = start()
	t.Cleanup(ns.Shutdown)
	waitDeclared("after the reconnect")

	if _, err := JetStream().Stream(ctx, "TEST_LV"); err != nil {
		t.Fatalf("stream missing after a wiped restart: %v", err)
	}
}

// RetryDeclare's backoff: a declare that fails while JetStream recovers is
// retried until it succeeds, and a canceled ctx ends the wait early.
func TestRetry(t *testing.T) {
	calls := 0
	err := retry(context.Background(), 5, time.Millisecond, func(context.Context) error {
		calls++
		if calls < 3 {
			return errors.New("jetstream not ready")
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("retry = (%v) after %d calls, want success on the 3rd", err, calls)
	}

	calls = 0
	err = retry(context.Background(), 3, time.Millisecond, func(context.Context) error {
		calls++
		return errors.New("still down")
	})
	if err == nil || calls != 3 {
		t.Fatalf("retry = (%v) after %d calls, want the last error after 3", err, calls)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls = 0
	start := time.Now()
	_ = retry(ctx, 5, time.Hour, func(context.Context) error {
		calls++
		return errors.New("down")
	})
	if calls != 1 || time.Since(start) > time.Second {
		t.Fatalf("retry on a canceled ctx made %d calls in %v, want 1 and no wait", calls, time.Since(start))
	}
}
