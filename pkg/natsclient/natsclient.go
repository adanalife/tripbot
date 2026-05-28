// Package natsclient owns the process-wide *nats.Conn for tripbot and
// onscreens-server. Mirrors the pkg/database singleton pattern: lazy on
// first Connect, no-op when the URL is empty, swappable for tests.
//
// Phase 1 (vault/tripbot/TODO.md "Adopt NATS as the inter-component
// message bus"): fire-and-forget pubsub. JetStream lands later.
package natsclient

import (
	"log/slog"
	"sync"

	"github.com/nats-io/nats.go"
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
