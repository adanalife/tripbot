package instrumentation

import (
	"context"
	"sync"
	"time"

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
	tripbotEvents      = mustCounter("tripbot_events_total", "Total login/logout events written to the events table, labeled by event")
	carSoundSelections = mustCounter("tripbot_carsound_selections_total", "Total times a viewer switched the YouTube background car-sound via !carsound, labeled by sound — drives the 'which voicing is most popular' question")
	scoreboardWrites   = mustCounter("tripbot_scoreboard_writes_total", "Total successful scoreboard score writes, labeled by scoreboard")
	twitchSubscribers  = mustGauge("twitch_subscribers_total", "Current number of Twitch channel subscribers")
	twitchFollowers    = mustGauge("twitch_followers_total", "Current number of Twitch channel followers")
	twitchConnected    = mustGauge("tripbot_twitch_connected", "1 when the bot is connected to Twitch chat (IRC), 0 otherwise")
	twitchTokenExpiry  = mustGauge("tripbot_twitch_token_expires_at_seconds", "Unix timestamp of the in-memory Twitch user-access-token's ExpiresAt, labeled by account (bot|broadcaster). 0 when the account has no loaded token.")
	twitchHelixErrors  = mustCounter("twitch_helix_errors_total", "Total non-2xx responses from the Twitch Helix API, labeled by endpoint and status_code")
	twitchChannelLive  = mustGauge("tripbot_twitch_channel_live", "1 when Helix GetStreams reports the configured channel as live, 0 when offline. Driven by the OBS silent-disconnect watchdog's Helix poll.")
	currentState       = mustGauge("tripbot_current_state", "1 for the US state the dashcam playhead is currently in, 0 for the previously-active state, labeled by state (2-letter abbrev, or \"unknown\"). Only one series reads 1 at a time. Drives the states-visited heatmap and the 'stuck on unknown' alert.")

	gatewayUp = mustGauge("tripbot_gateway_up", "1 when tripbot's last platform-gateway call got an HTTP response (gateway reachable), 0 when it failed at the transport layer (connection refused, timeout, DNS). Consumer-side reachability — paired with the gateway's own platform_gateway_up (process liveness).")

	obsSilentDisconnectRestarts = mustCounter("tripbot_obs_silent_disconnect_restarts_total", "Total times the OBS silent-disconnect watchdog forced a StopStream+StartStream because OBS reported outputActive=true while Twitch reported the channel offline")

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

// CarSoundSelections exposes the !carsound popularity counter. Record by
// calling CarSoundSelections.Inc(soundName) when a viewer switches the
// background car-sound, so Grafana can rank voicings by selection count.
var CarSoundSelections = carSoundSelectionsIface{counter: carSoundSelections}

// TwitchAudience exposes subscriber and follower gauge recording.
var TwitchAudience = twitchAudienceIface{subscribers: twitchSubscribers, followers: twitchFollowers}

// TwitchConnection exposes the chat-connection gauge. Set(true) on IRC
// connect, Set(false) on disconnect. Readiness doesn't gate on the Twitch
// connection (the pod stays in the Service so the re-auth page is reachable),
// so this gauge — alongside the admin-panel status row — is what surfaces
// "up but not in chat" to dashboards and alerts.
var TwitchConnection = twitchConnectionIface{gauge: twitchConnected}

// TwitchTokenExpiry exposes the per-account token-expiry timestamp gauge.
// SetExpiresAt(account, t) records t.Unix(), or 0 if t is the zero Time —
// the latter is how a blanked or never-loaded token shows up. Drives the
// "tripbot needs reauth" alert (time() past the recorded expiry).
var TwitchTokenExpiry = twitchTokenExpiryIface{gauge: twitchTokenExpiry}

// TwitchHelixErrors exposes the helix-error counter; record by calling
// TwitchHelixErrors.Inc(endpoint, statusCode). Endpoint is a short label
// like "GetUsers"; statusCode is the HTTP status reported by Twitch.
var TwitchHelixErrors = twitchHelixErrorsIface{counter: twitchHelixErrors}

// TwitchChannelLive exposes the per-tick Twitch live-status gauge written
// by the OBS silent-disconnect watchdog. Set(true) on every successful
// Helix poll that reports the channel as live, Set(false) when GetStreams
// returns empty. Paired with OBSStreaming in an alert: divergence
// (OBS=1 / Twitch=0) is the silent half-open RTMP signal.
var TwitchChannelLive = twitchChannelLiveIface{gauge: twitchChannelLive}

// CurrentState exposes the dashcam-state gauge. Call Set(abbrev) on every
// video transition with the active state's 2-letter abbreviation (or
// "unknown" when the playhead isn't over a resolvable US state). It sets the
// new state's series to 1 and clears the previously-active series to 0, so
// exactly one series reads 1 at any time and no stale =1 series linger for
// states the playhead has left.
var CurrentState = &currentStateIface{gauge: currentState}

// GatewayConnection exposes the consumer-side gateway-reachability gauge.
// Set(true) after any HTTP response from the platform-gateway, Set(false) on a
// transport failure (connection refused, timeout, DNS). Drives the "tripbot
// can't reach the gateway" alert — distinct from the gateway's own
// platform_gateway_up, which only reports that the gateway process is running.
var GatewayConnection = gatewayConnectionIface{gauge: gatewayUp}

// OBSSilentDisconnectRestarts exposes the watchdog's force-restart counter.
// Inc() is called after a successful StopStream+StartStream sequence.
// Any non-zero rate is alertable — the watchdog only fires after a
// 3-minute debounce, so even one increment means we saw a real silent
// disconnect in prod.
var OBSSilentDisconnectRestarts = obsSilentDisconnectRestartsIface{counter: obsSilentDisconnectRestarts}

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

type carSoundSelectionsIface struct{ counter metric.Int64Counter }

func (c carSoundSelectionsIface) Inc(sound string) {
	c.counter.Add(context.Background(), 1, metric.WithAttributes(attribute.String("sound", sound)))
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

type twitchTokenExpiryIface struct{ gauge metric.Int64Gauge }

func (t twitchTokenExpiryIface) SetExpiresAt(account string, expiresAt time.Time) {
	var v int64
	if !expiresAt.IsZero() {
		v = expiresAt.Unix()
	}
	t.gauge.Record(context.Background(), v, metric.WithAttributes(attribute.String("account", account)))
}

type twitchHelixErrorsIface struct{ counter metric.Int64Counter }

func (h twitchHelixErrorsIface) Inc(endpoint string, statusCode int) {
	h.counter.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String("endpoint", endpoint),
		attribute.Int("status_code", statusCode),
	))
}

type twitchChannelLiveIface struct{ gauge metric.Int64Gauge }

func (t twitchChannelLiveIface) Set(live bool) {
	var v int64
	if live {
		v = 1
	}
	t.gauge.Record(context.Background(), v)
}

type currentStateIface struct {
	gauge metric.Int64Gauge
	mu    sync.Mutex
	prev  string // last state set to 1, so we can clear it back to 0 on change
}

// Set records the active dashcam state. A blank abbrev is normalized to
// "unknown" so the series always carries a non-empty label. On a transition
// it zeroes the previously-active series before setting the new one to 1; a
// repeated Set of the same state is a cheap no-op (the series already reads 1).
func (s *currentStateIface) Set(abbrev string) {
	if abbrev == "" {
		abbrev = "unknown"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if abbrev == s.prev {
		return
	}
	if s.prev != "" {
		s.gauge.Record(context.Background(), 0, metric.WithAttributes(attribute.String("state", s.prev)))
	}
	s.gauge.Record(context.Background(), 1, metric.WithAttributes(attribute.String("state", abbrev)))
	s.prev = abbrev
}

type gatewayConnectionIface struct{ gauge metric.Int64Gauge }

func (g gatewayConnectionIface) Set(reachable bool) {
	var v int64
	if reachable {
		v = 1
	}
	g.gauge.Record(context.Background(), v)
}

type obsSilentDisconnectRestartsIface struct{ counter metric.Int64Counter }

func (o obsSilentDisconnectRestartsIface) Inc() {
	o.counter.Add(context.Background(), 1)
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
// (typically the configured ServerType: "tripbot" / "onscreens_server").
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
