// Package natsclient owns the process-wide *nats.Conn for tripbot and
// onscreens-server. Mirrors the pkg/database singleton pattern: lazy on
// first Connect, no-op when the URL is empty, swappable for tests.
//
// Core NATS carries fire-and-forget pubsub; the JetStream accessor below
// serves durable, replayable streams — the admin live console backfills
// its chat/map buffers from them on startup.
package natsclient

import (
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

var (
	mu   sync.Mutex
	conn *nats.Conn
)

// reconnectWait is the delay between connect attempts, both for the initial
// connect (RetryOnFailedConnect) and for reconnects after a drop.
const reconnectWait = 2 * time.Second

// Connect dials the configured NATS endpoint and stashes the result as
// the package singleton. When url is empty the function returns nil and
// every downstream call no-ops — lets local dev / tests skip NATS.
// Idempotent: a successful prior Connect short-circuits on re-entry.
//
// The connection retries forever: RetryOnFailedConnect covers the boot race
// where this process starts before NATS is reachable (a node reboot brings
// apps and NATS up together), so a failed first dial no longer leaves the
// process permanently deaf. The returned conn is usable immediately —
// subscriptions made while disconnected are queued client-side and replayed
// on connect; publishes buffer up to the client's reconnect buffer and are
// otherwise dropped, matching the fire-and-forget contract.
//
// onConnect callbacks run once, when the connection is first established
// (immediately or after retries) — the place for JetStream stream declares
// and anything else that needs a live server.
func Connect(url string, name string, onConnect ...func(*nats.Conn)) *nats.Conn {
	mu.Lock()
	defer mu.Unlock()
	if conn != nil {
		return conn
	}
	if url == "" {
		slog.Info("nats disabled (NATS_URL unset); pubsub publishes will no-op")
		return nil
	}
	c, err := nats.Connect(url,
		nats.Name(name),
		nats.MaxReconnects(-1),
		nats.RetryOnFailedConnect(true),
		nats.ReconnectWait(reconnectWait),
		nats.ConnectHandler(func(nc *nats.Conn) {
			slog.Info("nats connected", "url", nc.ConnectedUrl(), "name", name)
			for _, f := range onConnect {
				f(nc)
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			slog.Info("nats reconnected", "url", nc.ConnectedUrl(), "name", name)
		}),
	)
	if err != nil {
		// Only non-retryable errors land here (bad URL/options) — dial
		// failures return a conn that keeps retrying in the background.
		slog.Error("nats config invalid; pubsub publishes will no-op", "err", err, "url", url)
		return nil
	}
	conn = c
	if !c.IsConnected() {
		slog.Warn("nats unreachable; retrying in background", "url", url, "name", name)
	}
	return conn
}

// Conn returns the current package-singleton *nats.Conn, or nil if
// Connect has not yet succeeded. Callers must nil-check.
func Conn() *nats.Conn {
	mu.Lock()
	defer mu.Unlock()
	return conn
}

// SetConn swaps the singleton for tests. Pair with a nats-server test
// fixture or a stubbed *nats.Conn.
func SetConn(c *nats.Conn) {
	mu.Lock()
	defer mu.Unlock()
	conn = c
}

// JetStream returns a JetStream context over the singleton connection, or nil
// when NATS is unconfigured (no Connect) or JetStream is unavailable on the
// server. Callers MUST nil-check and fall back to core NATS behavior — a server
// without JetStream enabled, or local dev with NATS_URL unset, both yield nil.
//
// jetstream.New only fails on a nil/closed conn (it does not round-trip to the
// server), so a non-nil return here means "we have a JS handle to try"; whether
// the account actually has JetStream surfaces on the first stream/consumer call.
func JetStream() jetstream.JetStream {
	c := Conn()
	if c == nil {
		return nil
	}
	js, err := jetstream.New(c)
	if err != nil {
		slog.Error("jetstream context init failed; durable streams disabled", "err", err)
		return nil
	}
	return js
}
