package instrumentation

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var meter = otel.Meter("github.com/adanalife/tripbot")

var (
	chatMessages        = mustCounter("tripbot_chat_messages", "The total number of chat messages")
	chatCommands        = mustCounter("tripbot_chat_commands", "The total number of chat commands")
	chatCommandDuration = mustHistogram(
		"tripbot_command_duration_seconds",
		"Chat command handler duration in seconds, labeled by command",
		// Standard Prometheus-style HTTP-latency buckets; covers fast in-memory
		// commands (helpCmd) up through slow DB-fanout commands (milesCmd with
		// the 4-query GetScore chain).
		0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
	)
	tripbotEvents     = mustCounter("tripbot_events_total", "Total login/logout events written to the events table, labeled by event")
	scoreboardWrites  = mustCounter("tripbot_scoreboard_writes_total", "Total successful scoreboard score writes, labeled by scoreboard")
	twitchSubscribers = mustGauge("twitch_subscribers_total", "Current number of Twitch channel subscribers")
	twitchFollowers   = mustGauge("twitch_followers_total", "Current number of Twitch channel followers")
	twitchHelixErrors = mustCounter("twitch_helix_errors_total", "Total non-2xx responses from the Twitch Helix API, labeled by endpoint and status_code")
	obsStreamingGauge = mustGauge("obs_streaming_active", "1 if OBS is actively streaming, 0 otherwise")
)

// ChatMessages exposes the chat-message counter through a tiny stable API
// so call sites stay small (Inc()) and don't have to thread context.
var ChatMessages = chatCounterIface{counter: chatMessages}

// ChatCommands exposes the chat-command counter; record by calling
// ChatCommands.Inc(commandName).
var ChatCommands = chatCommandCounterIface{counter: chatCommands}

// ChatCommandDuration exposes the per-command latency histogram. Record by
// calling ChatCommandDuration.Observe(commandName, seconds) — typically with
// time.Since(start).Seconds() right after the handler returns.
var ChatCommandDuration = chatCommandDurationIface{h: chatCommandDuration}

// Events exposes the login/logout event counter. Record by calling
// Events.Inc("login") or Events.Inc("logout") right after the row is
// persisted.
var Events = eventsIface{counter: tripbotEvents}

// ScoreboardWrites exposes the scoreboard-write counter. Record by calling
// ScoreboardWrites.Inc(scoreboardName) right after the row is persisted.
var ScoreboardWrites = scoreboardWritesIface{counter: scoreboardWrites}

// TwitchAudience exposes subscriber and follower gauge recording.
var TwitchAudience = twitchAudienceIface{subscribers: twitchSubscribers, followers: twitchFollowers}

// TwitchHelixErrors exposes the helix-error counter; record by calling
// TwitchHelixErrors.Inc(endpoint, statusCode). Endpoint is a short label
// like "GetUsers"; statusCode is the HTTP status reported by Twitch.
var TwitchHelixErrors = twitchHelixErrorsIface{counter: twitchHelixErrors}

// OBSStreaming exposes the streaming-active gauge.
var OBSStreaming = obsStreamingIface{g: obsStreamingGauge}

type chatCounterIface struct{ counter metric.Int64Counter }

func (c chatCounterIface) Inc() {
	c.counter.Add(context.Background(), 1)
}

type chatCommandCounterIface struct{ counter metric.Int64Counter }

func (c chatCommandCounterIface) Inc(command string) {
	c.counter.Add(context.Background(), 1, metric.WithAttributes(attribute.String("command", command)))
}

type chatCommandDurationIface struct{ h metric.Float64Histogram }

func (d chatCommandDurationIface) Observe(command string, seconds float64) {
	d.h.Record(context.Background(), seconds, metric.WithAttributes(attribute.String("command", command)))
}

type eventsIface struct{ counter metric.Int64Counter }

func (e eventsIface) Inc(event string) {
	e.counter.Add(context.Background(), 1, metric.WithAttributes(attribute.String("event", event)))
}

type scoreboardWritesIface struct{ counter metric.Int64Counter }

func (s scoreboardWritesIface) Inc(scoreboard string) {
	s.counter.Add(context.Background(), 1, metric.WithAttributes(attribute.String("scoreboard", scoreboard)))
}

type twitchAudienceIface struct {
	subscribers metric.Int64Gauge
	followers   metric.Int64Gauge
}

func (a twitchAudienceIface) SetSubscribers(n int64) {
	a.subscribers.Record(context.Background(), n)
}

func (a twitchAudienceIface) SetFollowers(n int64) {
	a.followers.Record(context.Background(), n)
}

type twitchHelixErrorsIface struct{ counter metric.Int64Counter }

func (h twitchHelixErrorsIface) Inc(endpoint string, statusCode int) {
	h.counter.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String("endpoint", endpoint),
		attribute.Int("status_code", statusCode),
	))
}

type obsStreamingIface struct{ g metric.Int64Gauge }

func (o obsStreamingIface) Set(active bool) {
	v := int64(0)
	if active {
		v = 1
	}
	o.g.Record(context.Background(), v)
}

func mustCounter(name, desc string) metric.Int64Counter {
	c, err := meter.Int64Counter(name, metric.WithDescription(desc))
	if err != nil {
		panic(err)
	}
	return c
}

func mustGauge(name, desc string) metric.Int64Gauge {
	g, err := meter.Int64Gauge(name, metric.WithDescription(desc))
	if err != nil {
		panic(err)
	}
	return g
}

func mustHistogram(name, desc string, buckets ...float64) metric.Float64Histogram {
	opts := []metric.Float64HistogramOption{metric.WithDescription(desc)}
	if len(buckets) > 0 {
		opts = append(opts, metric.WithExplicitBucketBoundaries(buckets...))
	}
	h, err := meter.Float64Histogram(name, opts...)
	if err != nil {
		panic(err)
	}
	return h
}

func mustFloat64Gauge(name, desc string) metric.Float64Gauge {
	g, err := meter.Float64Gauge(name, metric.WithDescription(desc))
	if err != nil {
		panic(err)
	}
	return g
}
