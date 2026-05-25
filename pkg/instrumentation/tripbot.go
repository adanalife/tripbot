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
	twitchConnected   = mustGauge("tripbot_twitch_connected", "1 when the bot is connected to Twitch chat (IRC), 0 otherwise")
	twitchHelixErrors = mustCounter("twitch_helix_errors_total", "Total non-2xx responses from the Twitch Helix API, labeled by endpoint and status_code")

	twitchHelixRateRemaining = mustGauge("twitch_helix_rate_limit_remaining", "Last-seen Ratelimit-Remaining header from Twitch Helix responses (per app-access bearer)")
	twitchHelixRateLimit     = mustGauge("twitch_helix_rate_limit_total", "Last-seen Ratelimit-Limit header from Twitch Helix responses (per app-access bearer)")

	cronRuns     = mustCounter("tripbot_cron_runs_total", "Total cron job invocations, labeled by job")
	cronPanics   = mustCounter("tripbot_cron_panics_total", "Cron job panics recovered, labeled by job")
	cronLastRun  = mustGauge("tripbot_cron_last_run_timestamp_seconds", "Unix timestamp of the most recent completion of each cron job, labeled by job")
	cronDuration = mustHistogram(
		"tripbot_cron_duration_seconds",
		"Cron job duration in seconds, labeled by job",
		0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60,
	)

	httpPanics = mustCounter("tripbot_http_panics_total", "HTTP handler panics recovered, labeled by service")
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

// TwitchConnection exposes the chat-connection gauge. Set(true) on IRC
// connect, Set(false) on disconnect. Readiness no longer gates on the Twitch
// connection (the pod stays in the Service so the re-auth page is reachable),
// so this gauge — alongside the admin-panel status row — is what surfaces
// "up but not in chat" to dashboards and alerts.
var TwitchConnection = twitchConnectionIface{gauge: twitchConnected}

// TwitchHelixErrors exposes the helix-error counter; record by calling
// TwitchHelixErrors.Inc(endpoint, statusCode). Endpoint is a short label
// like "GetUsers"; statusCode is the HTTP status reported by Twitch.
var TwitchHelixErrors = twitchHelixErrorsIface{counter: twitchHelixErrors}

// TwitchHelixRateLimit exposes the per-bearer Helix rate-budget gauges.
// SetRemaining + SetLimit are called by the response-recording transport
// on every Helix call so dashboards / alerts can see headroom without
// waiting for a 429.
var TwitchHelixRateLimit = twitchHelixRateLimitIface{remaining: twitchHelixRateRemaining, limit: twitchHelixRateLimit}

// Cron exposes cron job metrics. Observe(job, seconds) is called on every
// completion (success or recovered panic); Panic(job) is additionally
// called when a recover() fires. Together they enable "stalled cron" and
// "panicking cron" alerts.
var Cron = cronIface{runs: cronRuns, panics: cronPanics, lastRun: cronLastRun, duration: cronDuration}

// HTTPPanics exposes the HTTP-handler panic counter. Increment from a
// recovery middleware that catches panics in the request goroutine.
var HTTPPanics = httpPanicsIface{counter: httpPanics}

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

type twitchConnectionIface struct{ gauge metric.Int64Gauge }

func (t twitchConnectionIface) Set(connected bool) {
	var v int64
	if connected {
		v = 1
	}
	t.gauge.Record(context.Background(), v)
}

type twitchHelixErrorsIface struct{ counter metric.Int64Counter }

func (h twitchHelixErrorsIface) Inc(endpoint string, statusCode int) {
	h.counter.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String("endpoint", endpoint),
		attribute.Int("status_code", statusCode),
	))
}

type twitchHelixRateLimitIface struct {
	remaining metric.Int64Gauge
	limit     metric.Int64Gauge
}

func (r twitchHelixRateLimitIface) SetRemaining(n int64) {
	r.remaining.Record(context.Background(), n)
}

func (r twitchHelixRateLimitIface) SetLimit(n int64) {
	r.limit.Record(context.Background(), n)
}

type cronIface struct {
	runs     metric.Int64Counter
	panics   metric.Int64Counter
	lastRun  metric.Int64Gauge
	duration metric.Float64Histogram
}

// Observe records a completed cron run: bumps the run counter, records the
// duration, and updates the last-run timestamp. Call on every completion,
// including when a panic was recovered, so "no successful run in 3× interval"
// alerts still see activity from a panicking job.
func (c cronIface) Observe(job string, seconds float64, now int64) {
	attr := metric.WithAttributes(attribute.String("job", job))
	c.runs.Add(context.Background(), 1, attr)
	c.duration.Record(context.Background(), seconds, attr)
	c.lastRun.Record(context.Background(), now, attr)
}

// Panic records a cron panic. Call from a recover() handler before Observe.
func (c cronIface) Panic(job string) {
	c.panics.Add(context.Background(), 1, metric.WithAttributes(attribute.String("job", job)))
}

type httpPanicsIface struct{ counter metric.Int64Counter }

// Inc records one recovered HTTP-handler panic, labeled by service
// (typically c.Conf.ServerType: "tripbot" / "vlc_server" / "onscreens_server").
func (h httpPanicsIface) Inc(service string) {
	h.counter.Add(context.Background(), 1, metric.WithAttributes(attribute.String("service", service)))
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
