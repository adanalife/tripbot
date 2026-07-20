// Package natsclient owns the process-wide *nats.Conn for tripbot and
// onscreens-server. Mirrors the pkg/database singleton pattern: lazy on
// first Connect, no-op when the URL is empty, swappable for tests.
//
// Core NATS carries fire-and-forget pubsub; the JetStream accessor below
// serves durable, replayable streams — the admin live console backfills
// its chat/map buffers from them on startup.
package natsclient

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	mu        sync.Mutex
	conn      *nats.Conn
	gaugeOnce sync.Once
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
	gaugeOnce.Do(registerConnectedGauge)
	if !c.IsConnected() {
		slog.Warn("nats unreachable; retrying in background", "url", url, "name", name)
	}
	return conn
}

// registerConnectedGauge wires up nats_connected, an observable gauge that
// reports 1 while the singleton connection is established and 0 while it's
// disconnected, reconnecting, or never came up. A sustained 0 means this
// process's fire-and-forget publishes are dropped and its subscriptions aren't
// delivering — the silent failure mode where a consumer boots before NATS is
// reachable and stays deaf. Registered once, on the first successful Connect;
// the callback reads the live conn each collection, so it tracks drops and
// recoveries without any handler bookkeeping. Uses the global meter provider,
// which pkg/telemetry sets up (and which late-binds instruments created before
// that, so registration order with telemetry.Init doesn't matter).
func registerConnectedGauge() {
	// Stamp service.platform onto the datapoint so the per-platform instances
	// (tripbot-twitch/youtube, onscreens-twitch/youtube) don't collide onto one
	// byte-identical series: service.platform on the OTel resource reaches only
	// target_info, not datapoints. Mirrors pkg/instrumentation's platformAttr
	// and the service-health dashboards' service_platform filter; defaults to
	// twitch like the config default.
	platform := os.Getenv("STREAM_PLATFORM")
	if platform == "" {
		platform = "twitch"
	}
	platformAttr := metric.WithAttributes(attribute.String("service.platform", platform))

	meter := otel.Meter("github.com/adanalife/tripbot/pkg/natsclient")
	_, err := meter.Int64ObservableGauge(
		"nats_connected",
		metric.WithDescription("1 while the process's NATS connection is established, 0 while disconnected/reconnecting or never connected (publishes dropped, subscriptions not delivering)"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			var v int64
			if c := Conn(); c != nil && c.IsConnected() {
				v = 1
			}
			o.Observe(v, platformAttr)
			return nil
		}),
	)
	if err != nil {
		slog.Error("register nats_connected gauge failed", "err", err)
	}
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
