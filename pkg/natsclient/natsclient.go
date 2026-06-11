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

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

var (
	mu   sync.Mutex
	conn *nats.Conn
)

// Connect dials the configured NATS endpoint and stashes the result as
// the package singleton. When url is empty the function returns nil and
// every downstream call no-ops — lets local dev / tests skip NATS.
// Idempotent: a successful prior Connect short-circuits on re-entry.
func Connect(url string, name string) *nats.Conn {
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
	)
	if err != nil {
		slog.Error("nats connect failed; pubsub publishes will no-op", "err", err, "url", url)
		return nil
	}
	conn = c
	slog.Info("nats connected", "url", url, "name", name)
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
