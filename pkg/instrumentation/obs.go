package instrumentation

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// platformAttr stamps the streaming platform (twitch/youtube) onto every
// OBS metric series. Without it the per-platform instances emit
// byte-identical series identities — service.platform lives only on the
// OTel resource → target_info, not on the datapoints — so the twitch and
// youtube encoders collide onto one series. The attribute key matches the
// resource attribute (service.platform → service_platform label) so it
// lines up with target_info and the Stream Health dashboard's existing
// filters. Defaults to twitch to match the config default.
func platformAttr(platform string) metric.MeasurementOption {
	if platform == "" {
		platform = "twitch"
	}
	return metric.WithAttributes(attribute.String("service.platform", platform))
}

var (
	obsStreamingGauge         = mustGauge("obs_streaming_active", "1 if OBS is actively streaming, 0 otherwise")
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
	OutputBytes      float64
	OutputDurationMS float64
	OutputCongestion float64
	Reconnecting     bool
	SkippedFrames    float64
	TotalFrames      float64
}

// OBSStats publishes the OBS streaming-active, performance, and
// stream-output gauges, stamping every series with the streaming platform
// it was constructed with.
type OBSStats struct {
	platform metric.MeasurementOption
}

// NewOBSStats builds a publisher for the given streaming platform
// (the instance's STREAM_PLATFORM value).
func NewOBSStats(platform string) OBSStats {
	return OBSStats{platform: platformAttr(platform)}
}

// SetStreaming records whether OBS is actively streaming.
func (o OBSStats) SetStreaming(active bool) {
	v := int64(0)
	if active {
		v = 1
	}
	obsStreamingGauge.Record(context.Background(), v, o.platform)
}

func (o OBSStats) Update(s OBSStatsSnapshot) {
	ctx := context.Background()
	obsActiveFPS.Record(ctx, s.ActiveFPS, o.platform)
	obsAverageFrameRenderMS.Record(ctx, s.AverageFrameRenderTime, o.platform)
	obsCPUUsage.Record(ctx, s.CPUUsage, o.platform)
	obsMemoryUsage.Record(ctx, s.MemoryUsage, o.platform)
	obsRenderSkippedFrames.Record(ctx, s.RenderSkippedFrames, o.platform)
	obsRenderTotalFrames.Record(ctx, s.RenderTotalFrames, o.platform)
	obsOutputSkippedFrames.Record(ctx, s.OutputSkippedFrames, o.platform)
	obsOutputTotalFrames.Record(ctx, s.OutputTotalFrames, o.platform)
}

func (o OBSStats) UpdateStream(s OBSStreamSnapshot) {
	ctx := context.Background()
	obsStreamOutputBytes.Record(ctx, s.OutputBytes, o.platform)
	obsStreamOutputDurationMS.Record(ctx, s.OutputDurationMS, o.platform)
	obsStreamCongestion.Record(ctx, s.OutputCongestion, o.platform)
	v := int64(0)
	if s.Reconnecting {
		v = 1
	}
	obsStreamReconnecting.Record(ctx, v, o.platform)
	obsStreamSkippedFrames.Record(ctx, s.SkippedFrames, o.platform)
	obsStreamTotalFrames.Record(ctx, s.TotalFrames, o.platform)
}
