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

	obsActiveFPS              = mustFloat64Gauge("obs_active_fps", "Current FPS being rendered by OBS")
	obsAverageFrameRenderMS   = mustFloat64Gauge("obs_average_frame_render_time_ms", "Average time in milliseconds OBS spends rendering a frame")
	obsCPUUsage               = mustFloat64Gauge("obs_cpu_usage_percent", "Current OBS CPU usage (percent)")
	obsMemoryUsage            = mustFloat64Gauge("obs_memory_usage_mb", "Current OBS memory usage in MB")
	obsRenderSkippedFrames    = mustFloat64Gauge("obs_render_skipped_frames", "Render-thread skipped frames since OBS started")
	obsRenderTotalFrames      = mustFloat64Gauge("obs_render_total_frames", "Render-thread total frames since OBS started")
	obsOutputSkippedFrames    = mustFloat64Gauge("obs_output_skipped_frames", "Output-thread skipped frames since OBS started")
	obsOutputTotalFrames      = mustFloat64Gauge("obs_output_total_frames", "Output-thread total frames since OBS started")
	obsStreamOutputBytes      = mustFloat64Gauge("obs_stream_output_bytes", "Bytes sent by the stream output (cumulative since stream start)")
	obsStreamOutputDurationMS = mustFloat64Gauge("obs_stream_output_duration_ms", "Current stream output duration in milliseconds")
	obsStreamCongestion       = mustFloat64Gauge("obs_stream_output_congestion", "Stream output congestion (0..1)")
	obsStreamReconnecting     = mustGauge("obs_stream_output_reconnecting", "1 if the stream output is currently reconnecting, 0 otherwise")
	obsStreamSkippedFrames    = mustFloat64Gauge("obs_stream_output_skipped_frames", "Stream-output skipped frames since stream start")
	obsStreamTotalFrames      = mustFloat64Gauge("obs_stream_output_total_frames", "Stream-output total frames since stream start")
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

// OBSStatsSnapshot bundles OBS performance + stream-output stats so the
// poller can publish in a single call without coupling instrumentation to
// goobs types.
type OBSStatsSnapshot struct {
	ActiveFPS              float64
	AverageFrameRenderTime float64 // ms
	CPUUsage               float64 // percent
	MemoryUsage            float64 // MB
	RenderSkippedFrames    float64
	RenderTotalFrames      float64
	OutputSkippedFrames    float64
	OutputTotalFrames      float64
}

// OBSStreamSnapshot is the stream-output side (only meaningful while
// streaming, but always safe to publish — fields are zero when idle).
type OBSStreamSnapshot struct {
	OutputBytes       float64
	OutputDurationMS  float64
	OutputCongestion  float64
	Reconnecting      bool
	SkippedFrames     float64
	TotalFrames       float64
}

// OBSStats exposes the OBS performance + stream-output gauges.
var OBSStats = obsStatsIface{
	activeFPS:           obsActiveFPS,
	averageFrameRender:  obsAverageFrameRenderMS,
	cpuUsage:            obsCPUUsage,
	memoryUsage:         obsMemoryUsage,
	renderSkippedFrames: obsRenderSkippedFrames,
	renderTotalFrames:   obsRenderTotalFrames,
	outputSkippedFrames: obsOutputSkippedFrames,
	outputTotalFrames:   obsOutputTotalFrames,
	streamBytes:         obsStreamOutputBytes,
	streamDuration:      obsStreamOutputDurationMS,
	streamCongestion:    obsStreamCongestion,
	streamReconnecting:  obsStreamReconnecting,
	streamSkipped:       obsStreamSkippedFrames,
	streamTotal:         obsStreamTotalFrames,
}

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

type obsStatsIface struct {
	activeFPS           metric.Float64Gauge
	averageFrameRender  metric.Float64Gauge
	cpuUsage            metric.Float64Gauge
	memoryUsage         metric.Float64Gauge
	renderSkippedFrames metric.Float64Gauge
	renderTotalFrames   metric.Float64Gauge
	outputSkippedFrames metric.Float64Gauge
	outputTotalFrames   metric.Float64Gauge
	streamBytes         metric.Float64Gauge
	streamDuration      metric.Float64Gauge
	streamCongestion    metric.Float64Gauge
	streamReconnecting  metric.Int64Gauge
	streamSkipped       metric.Float64Gauge
	streamTotal         metric.Float64Gauge
}

func (o obsStatsIface) Update(s OBSStatsSnapshot) {
	ctx := context.Background()
	o.activeFPS.Record(ctx, s.ActiveFPS)
	o.averageFrameRender.Record(ctx, s.AverageFrameRenderTime)
	o.cpuUsage.Record(ctx, s.CPUUsage)
	o.memoryUsage.Record(ctx, s.MemoryUsage)
	o.renderSkippedFrames.Record(ctx, s.RenderSkippedFrames)
	o.renderTotalFrames.Record(ctx, s.RenderTotalFrames)
	o.outputSkippedFrames.Record(ctx, s.OutputSkippedFrames)
	o.outputTotalFrames.Record(ctx, s.OutputTotalFrames)
}

func (o obsStatsIface) UpdateStream(s OBSStreamSnapshot) {
	ctx := context.Background()
	o.streamBytes.Record(ctx, s.OutputBytes)
	o.streamDuration.Record(ctx, s.OutputDurationMS)
	o.streamCongestion.Record(ctx, s.OutputCongestion)
	v := int64(0)
	if s.Reconnecting {
		v = 1
	}
	o.streamReconnecting.Record(ctx, v)
	o.streamSkipped.Record(ctx, s.SkippedFrames)
	o.streamTotal.Record(ctx, s.TotalFrames)
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
