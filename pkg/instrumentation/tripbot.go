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
		// commands (commandsCmd) up through slow DB-fanout commands (milesCmd with
		// the 4-query GetScore chain).
		0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
	)
	tripbotEvents             = mustCounter("tripbot_events_total", "Total rows written to the events table, labeled by event and by service_platform. Every kind in the event taxonomy counts here — logins, logouts, follows, command runs, deploys, watchdog transitions, state crossings.")
	announcements             = mustCounter("tripbot_announcements_total", "Total viewer-milestone shouts posted to chat, labeled by kind (follow|sub|gift|resub) and by service_platform. Pairs with tripbot_events_total to cover the one leg of the EventSub path nothing else watches: a notice that persists its row but never lands a shout means outbound chat is wedged. A shout's only other trace is ordinary bot chat text, which no query can tell apart from any other line the bot says without matching on its wording.")
	backgroundAudioSelections = mustCounter("tripbot_background_audio_selections_total", "Total background-audio bed switches, labeled by bed (somafm|carhum|album) and platform — answers what the stream has been playing and how often it changes")
	scoreboardWrites          = mustCounter("tripbot_scoreboard_writes_total", "Total successful scoreboard score writes, labeled by scoreboard")
	twitchSubscribers         = mustGauge("twitch_subscribers_total", "Current number of Twitch channel subscribers")
	twitchFollowers           = mustGauge("twitch_followers_total", "Current number of Twitch channel followers")
	twitchConnected           = mustGauge("tripbot_twitch_connected", "1 when the bot is receiving Twitch chat, 0 otherwise")
	twitchTokenExpiry         = mustGauge("tripbot_twitch_token_expires_at_seconds", "Unix timestamp of the in-memory Twitch user-access-token's ExpiresAt, labeled by account (bot|broadcaster). 0 when the account has no loaded token.")
	channelLive               = mustGauge("tripbot_channel_live", "1 when the platform reports this instance's channel as live, 0 when offline, labeled by service_platform. Paired with obs_streaming_active in the silent-disconnect alert: OBS=1 while the platform says 0 means we are streaming into the void.")
	currentState              = mustGauge("tripbot_current_state", "1 for the US state the dashcam playhead is currently in, 0 for the previously-active state, labeled by state (2-letter abbrev, or \"unknown\"). Only one series reads 1 at a time. Drives the states-visited heatmap and the 'stuck on unknown' alert.")

	eventsubSubscriptions = mustGauge("tripbot_eventsub_subscriptions", "Twitch EventSub subscriptions the current session holds (result=ok) and the ones Twitch refused on the last subscribe round (result=denied), labeled by service_platform. result=ok at 0 means no real-time follow/subscribe/raid events are arriving at all; result=denied above 0 with result=ok also above 0 is a partial grant, where only the event types needing the missing scope are dead.")

	gatewayUp = mustGauge("tripbot_gateway_up", "1 when tripbot's last platform-gateway call got an HTTP response (gateway reachable), 0 when it failed at the transport layer (connection refused, timeout, DNS). Consumer-side reachability — paired with the gateway's own platform_gateway_up (process liveness).")

	obsSilentDisconnectRestarts = mustCounter("tripbot_obs_silent_disconnect_restarts_total", "Total recoveries the silent-disconnect watchdog attempted because OBS reported outputActive=true while the platform reported the channel offline, labeled by service_platform and by result (ok, failed). The recovery is a StopStream+StartStream on Twitch and YouTube and an egress re-mint on TikTok")
	obsRecoveryExhausted        = mustGauge("tripbot_obs_recovery_exhausted", "1 while the silent-disconnect watchdog has stood down on a platform: it forced its maximum run of consecutive recoveries and the channel stayed offline through every one, so the fault is upstream of anything a restart can fix and it has stopped bouncing the output. 0 otherwise, labeled by service_platform. Clears when the channel comes back or the OBS output is stopped. Drives the 'watchdog exhausted' alert.")

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
var ChatMessages = chatMessagesCounter{counter: chatMessages}

// ChatCommands exposes the chat-command counter; record by calling
// ChatCommands.Inc(commandName).
var ChatCommands = chatCommandsCounter{counter: chatCommands}

// ChatCommandDuration exposes the per-command latency histogram. Record by
// calling ChatCommandDuration.Observe(commandName, seconds) — typically with
// time.Since(start).Seconds() right after the handler returns.
var ChatCommandDuration = commandDurationHistogram{h: chatCommandDuration}

// Events exposes the events-table counter. Record by calling
// Events.Inc(event, platform) right after the row is persisted — the platform
// is stamped as a datapoint attribute because service.platform lives only on
// the OTel resource, so without it a dashboard cannot break the rate down by
// encoder and a per-platform selector silently matches nothing.
var Events = eventsCounter{counter: tripbotEvents}

// Announcements exposes the chat-shout counter. Record by calling
// Announcements.Inc(platform, kind) right after the Say, so the count reflects
// what was actually handed to the outbound client rather than what an EventSub
// notice asked for.
var Announcements = announcementsCounter{counter: announcements}

// ScoreboardWrites exposes the scoreboard-write counter. Record by calling
// ScoreboardWrites.Inc(scoreboardName) right after the row is persisted.
var ScoreboardWrites = scoreboardWritesCounter{counter: scoreboardWrites}

// BackgroundAudioSelections exposes the bed-switch counter. Recorded inside
// beds.Store.Set, so every switch counts once no matter which surface — the
// console or !audio — asked for it.
var BackgroundAudioSelections = backgroundAudioSelectionsCounter{counter: backgroundAudioSelections}

// TwitchAudience exposes subscriber and follower gauge recording.
var TwitchAudience = twitchAudienceGauges{subscribers: twitchSubscribers, followers: twitchFollowers}

// TwitchConnection exposes the chat-connection gauge — whether the bot can
// reach Twitch chat, written by the gateway inbound poll (the gateway holds the
// chat transport itself). Readiness doesn't gate on the Twitch connection (the
// pod stays in the Service so the re-auth page is reachable), so this gauge —
// alongside the admin-panel status row — is what surfaces "up but not in chat"
// to dashboards and alerts.
//
// It pairs with the gateway's own platform_gateway_chat_connected: this one 0
// while that one is 1 localises the fault to the path between them.
var TwitchConnection = twitchConnectionGauge{gauge: twitchConnected}

// TwitchTokenExpiry exposes the per-account token-expiry timestamp gauge.
// SetExpiresAt(account, t) records t.Unix(), or 0 if t is the zero Time —
// the latter is how a blanked or never-loaded token shows up. Drives the
// "tripbot needs reauth" alert (time() past the recorded expiry).
var TwitchTokenExpiry = twitchTokenExpiryGauge{gauge: twitchTokenExpiry}

// ChannelLive exposes the platform-agnostic live-status gauge. Call
// Set(live, platform) from whatever already learns this instance's live state —
// the Twitch watchdog's Helix poll, the gateway inbound-chat poll's Live flag,
// or a broadcast-discovery tick. The platform is stamped as a datapoint
// attribute for the same reason the OBS metrics do it: service.platform lives
// only on the resource, so without it every per-platform instance would emit the
// same series identity and collide onto one, last write winning. Every platform
// an alert can watch must call this; a platform that never does has no series,
// which the lost-visibility canary reports rather than reading as offline.
var ChannelLive = channelLiveGauge{gauge: channelLive}

// CurrentState exposes the dashcam-state gauge. Call Set(abbrev, platform) on
// every video transition with the active state's 2-letter abbreviation (or
// "unknown" when the playhead isn't over a resolvable US state) and the
// instance's streaming platform. It sets the new state's series to 1 and
// clears the previously-active series to 0, so exactly one series reads 1 at
// any time and no stale =1 series linger for states the playhead has left.
var CurrentState = &currentStateGauge{gauge: currentState}

// EventSubSubscriptions exposes the EventSub session's subscription count.
// Set(held, denied) is called after every subscribe round and again when the
// session ends with held=0, so the gauge always answers "are real-time events
// arriving right now?" rather than "did they ever arrive".
//
// A positive liveness signal is the point. The events themselves — follows,
// subs, raids — are far too sparse to alert on: a flat zero for hours is the
// normal reading, so their absence cannot distinguish a quiet channel from a
// dead subscription. On 2026-08-18 EventSub was down on prod for 7½ hours with
// the pod Ready, tripbot_channel_live at 1 and no rule firing.
//
// The twitch instance is the only caller (EventSub is Twitch-only), so the
// platform attribute is stamped here rather than threaded through pkg/eventsub.
var EventSubSubscriptions = eventsubSubscriptionsGauge{gauge: eventsubSubscriptions}

// GatewayConnection exposes the consumer-side gateway-reachability gauge.
// Set(true) after any HTTP response from the platform-gateway, Set(false) on a
// transport failure (connection refused, timeout, DNS). Drives the "tripbot
// can't reach the gateway" alert — distinct from the gateway's own
// platform_gateway_up, which only reports that the gateway process is running.
var GatewayConnection = gatewayConnectionGauge{gauge: gatewayUp}

// OBSSilentDisconnectRestarts exposes the watchdog's forced-recovery counter.
// Attempt is called once per recovery the watchdog forces, whether or not it
// worked. Any non-zero rate is alertable — the watchdog only fires after a
// multi-minute debounce, so even one increment means we saw a real silent
// disconnect in prod.
//
// Counting attempts rather than successes is the load-bearing part. Recovery
// that keeps failing is the worse outage and the one that used to be invisible:
// through a 9h41m outage on 2026-08-05 the watchdog attempted a restart every
// 60s and failed every time, and this counter never moved, so the panel built
// on it read a flat zero for the whole incident.
var OBSSilentDisconnectRestarts = obsSilentDisconnectRestartsCounter{counter: obsSilentDisconnectRestarts}

// OBSRecoveryExhausted exposes the watchdog's stood-down state. Set(true,
// platform) when the watchdog stops forcing recoveries because a full run of
// them changed nothing; Set(false, platform) at startup and whenever the
// channel comes back or the output stops, so the series exists (an alert can
// read 0) and clears on its own once the fault is gone.
//
// A gauge rather than a counter result because the alert wants the *state*:
// on 2026-08-23/24 the watchdog bounced YouTube every 10 minutes for 17 hours
// and every bounce was a no-op, so "still stood down" has to stay readable for
// as long as it is true, not only within a window of the moment it happened.
var OBSRecoveryExhausted = obsRecoveryExhaustedGauge{gauge: obsRecoveryExhausted}

// Cron exposes cron job metrics. Observe(job, seconds) is called on every
// completion (success or recovered panic); Panic(job) is additionally
// called when a recover() fires. Together they enable "stalled cron" and
// "panicking cron" alerts.
var Cron = cronMetrics{runs: cronRuns, panics: cronPanics, lastRun: cronLastRun, duration: cronDuration}

// HTTPPanics exposes the HTTP-handler panic counter. Increment from a
// recovery middleware that catches panics in the request goroutine.
var HTTPPanics = httpPanicsCounter{counter: httpPanics}

type chatMessagesCounter struct{ counter metric.Int64Counter }

func (c chatMessagesCounter) Inc() {
	c.counter.Add(context.Background(), 1)
}

type chatCommandsCounter struct{ counter metric.Int64Counter }

func (c chatCommandsCounter) Inc(command string) {
	c.counter.Add(context.Background(), 1, metric.WithAttributes(attribute.String("command", command)))
}

type commandDurationHistogram struct{ h metric.Float64Histogram }

func (d commandDurationHistogram) Observe(command string, seconds float64) {
	d.h.Record(context.Background(), seconds, metric.WithAttributes(attribute.String("command", command)))
}

type eventsCounter struct{ counter metric.Int64Counter }

func (e eventsCounter) Inc(event, platform string) {
	e.counter.Add(context.Background(), 1,
		metric.WithAttributes(attribute.String("event", event)), platformAttr(platform))
}

type announcementsCounter struct{ counter metric.Int64Counter }

func (a announcementsCounter) Inc(platform, kind string) {
	a.counter.Add(context.Background(), 1,
		metric.WithAttributes(attribute.String("kind", kind)), platformAttr(platform))
}

type scoreboardWritesCounter struct{ counter metric.Int64Counter }

func (s scoreboardWritesCounter) Inc(scoreboard string) {
	s.counter.Add(context.Background(), 1, metric.WithAttributes(attribute.String("scoreboard", scoreboard)))
}

type backgroundAudioSelectionsCounter struct{ counter metric.Int64Counter }

func (c backgroundAudioSelectionsCounter) Inc(platform, bed string) {
	c.counter.Add(context.Background(), 1, metric.WithAttributes(attribute.String("bed", bed)), platformAttr(platform))
}

type twitchAudienceGauges struct {
	subscribers metric.Int64Gauge
	followers   metric.Int64Gauge
}

func (a twitchAudienceGauges) SetSubscribers(n int64) {
	a.subscribers.Record(context.Background(), n)
}

func (a twitchAudienceGauges) SetFollowers(n int64) {
	a.followers.Record(context.Background(), n)
}

type twitchConnectionGauge struct{ gauge metric.Int64Gauge }

func (t twitchConnectionGauge) Set(connected bool) {
	t.gauge.Record(context.Background(), b2i(connected))
}

type twitchTokenExpiryGauge struct{ gauge metric.Int64Gauge }

func (t twitchTokenExpiryGauge) SetExpiresAt(account string, expiresAt time.Time) {
	var v int64
	if !expiresAt.IsZero() {
		v = expiresAt.Unix()
	}
	t.gauge.Record(context.Background(), v, metric.WithAttributes(attribute.String("account", account)))
}

type channelLiveGauge struct{ gauge metric.Int64Gauge }

func (c channelLiveGauge) Set(live bool, platform string) {
	c.gauge.Record(context.Background(), b2i(live), platformAttr(platform))
}

type currentStateGauge struct {
	gauge metric.Int64Gauge
	mu    sync.Mutex
	prev  string // last state set to 1, so we can clear it back to 0 on change
}

// Set records the active dashcam state for the given streaming platform. A
// blank abbrev is normalized to "unknown" so the series always carries a
// non-empty label. The platform is stamped as a datapoint attribute
// (service.platform) so the per-platform instances don't collide on a
// byte-identical series — matching the OBS gauges and target_info. On a
// transition it zeroes the previously-active series before setting the new one
// to 1; a repeated Set of the same state is a cheap no-op (the series already
// reads 1).
func (s *currentStateGauge) Set(abbrev, platform string) {
	if abbrev == "" {
		abbrev = "unknown"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if abbrev == s.prev {
		return
	}
	plat := platformAttr(platform)
	if s.prev != "" {
		s.gauge.Record(context.Background(), 0, metric.WithAttributes(attribute.String("state", s.prev)), plat)
	}
	s.gauge.Record(context.Background(), 1, metric.WithAttributes(attribute.String("state", abbrev)), plat)
	s.prev = abbrev
}

type eventsubSubscriptionsGauge struct{ gauge metric.Int64Gauge }

// Set records both legs of one subscribe round. denied is carried through the
// session-ended call rather than zeroed with held, because it describes the last
// round's verdict on the token: zeroing it on every socket drop would blink a
// standing scope shortfall out of the series once an hour.
func (e eventsubSubscriptionsGauge) Set(held, denied int) {
	e.record("ok", held)
	e.record("denied", denied)
}

func (e eventsubSubscriptionsGauge) record(result string, n int) {
	e.gauge.Record(context.Background(), int64(n),
		platformAttr("twitch"),
		metric.WithAttributes(attribute.String("result", result)),
	)
}

type gatewayConnectionGauge struct{ gauge metric.Int64Gauge }

func (g gatewayConnectionGauge) Set(reachable bool) {
	g.gauge.Record(context.Background(), b2i(reachable))
}

type obsSilentDisconnectRestartsCounter struct{ counter metric.Int64Counter }

// Attempt records one forced recovery, labeled by platform and by whether it
// landed. Takes the restart error rather than a bool so the call site can't
// record success on a path that errored — the ordering bug this replaced.
func (o obsSilentDisconnectRestartsCounter) Attempt(platform string, restartErr error) {
	result := "ok"
	if restartErr != nil {
		result = "failed"
	}
	o.counter.Add(context.Background(), 1,
		platformAttr(platform),
		metric.WithAttributes(attribute.String("result", result)),
	)
}

type obsRecoveryExhaustedGauge struct{ gauge metric.Int64Gauge }

func (o obsRecoveryExhaustedGauge) Set(exhausted bool, platform string) {
	o.gauge.Record(context.Background(), b2i(exhausted), platformAttr(platform))
}

type cronMetrics struct {
	runs     metric.Int64Counter
	panics   metric.Int64Counter
	lastRun  metric.Int64Gauge
	duration metric.Float64Histogram
}

// Observe records a completed cron run: bumps the run counter, records the
// duration, and updates the last-run timestamp. Call on every completion,
// including when a panic was recovered, so "no successful run in 3× interval"
// alerts still see activity from a panicking job.
func (c cronMetrics) Observe(job string, seconds float64, now int64) {
	attr := metric.WithAttributes(attribute.String("job", job))
	c.runs.Add(context.Background(), 1, attr)
	c.duration.Record(context.Background(), seconds, attr)
	c.lastRun.Record(context.Background(), now, attr)
}

// Panic records a cron panic. Call from a recover() handler before Observe.
func (c cronMetrics) Panic(job string) {
	c.panics.Add(context.Background(), 1, metric.WithAttributes(attribute.String("job", job)))
}

type httpPanicsCounter struct{ counter metric.Int64Counter }

// Inc records one recovered HTTP-handler panic, labeled by service
// (typically the configured ServerType: "tripbot" / "onscreens_server").
func (h httpPanicsCounter) Inc(service string) {
	h.counter.Add(context.Background(), 1, metric.WithAttributes(attribute.String("service", service)))
}

// b2i maps a bool onto the 0/1 an OTel gauge records. Every liveness- and
// state-style gauge in the package is a bool at the call site and an Int64Gauge
// on the wire.
func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
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
