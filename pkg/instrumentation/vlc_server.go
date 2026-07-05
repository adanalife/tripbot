package instrumentation

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// platformAttr stamps the streaming platform (twitch/youtube) onto every
// vlc-server and OBS metric series. Without it the per-platform vlc-server
// instances emit byte-identical series identities — service.platform lives
// only on the OTel resource → target_info, not on the datapoints — so the
// twitch and youtube encoders collide onto one series. The attribute key
// matches the resource attribute (service.platform → service_platform label)
// so it lines up with target_info and the Stream Health dashboard's existing
// filters. Defaults to twitch to match the config default; cmd/vlc-server
// overrides it via SetPlatform at startup before the pollers begin.
var platformAttr = metric.WithAttributes(attribute.String("service.platform", "twitch"))

// SetPlatform stamps the streaming platform this instance feeds onto the
// vlc-server/OBS gauges. Call once at startup (before the stats pollers run)
// with the instance's STREAM_PLATFORM value so per-platform series stay
// distinct as more platforms are added.
func SetPlatform(p string) {
	if p == "" {
		p = "twitch"
	}
	platformAttr = metric.WithAttributes(attribute.String("service.platform", p))
}

var (
	vlcInputBitRate       = mustFloat64Gauge("vlc_player_input_bitrate", "libvlc input bitrate (libvlc-native units, ~bytes/µs)")
	vlcDemuxBitRate       = mustFloat64Gauge("vlc_player_demux_bitrate", "libvlc demux bitrate (libvlc-native units, ~bytes/µs)")
	vlcDisplayedFPS       = mustFloat64Gauge("vlc_player_displayed_fps", "Derived frames-per-second: delta of displayed pictures over the poll interval")
	vlcDecodedVideo       = mustFloat64Gauge("vlc_player_decoded_video_frames", "Total decoded video blocks since the current Media started (resets on media change)")
	vlcDisplayedPictures  = mustFloat64Gauge("vlc_player_displayed_pictures", "Total displayed frames since the current Media started (resets on media change)")
	vlcLostPictures       = mustFloat64Gauge("vlc_player_lost_pictures", "Total lost (dropped) frames since the current Media started (resets on media change)")
	vlcDemuxCorrupted     = mustFloat64Gauge("vlc_player_demux_corrupted", "Demux corruptions discarded since the current Media started")
	vlcDemuxDiscontinuity = mustFloat64Gauge("vlc_player_demux_discontinuity", "Demux discontinuities dropped since the current Media started")

	vlcTimeRemaining = mustFloat64Gauge("vlc_player_time_remaining_seconds", "Seconds remaining in the currently-playing clip (media length minus current playhead position)")
	vlcProgress      = mustFloat64Gauge("vlc_player_progress_fraction", "Playback progress through the currently-playing clip as a 0..1 fraction")

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

// VLCPlayerStatsSnapshot is the shape vlc-server hands to instrumentation
// each poll tick. Decouples the gauges from libvlc-go's MediaStats type.
type VLCPlayerStatsSnapshot struct {
	InputBitRate       float64
	DemuxBitRate       float64
	DisplayedFPS       float64 // derived by the caller from delta of DisplayedPictures
	DecodedVideo       float64
	DisplayedPictures  float64
	LostPictures       float64
	DemuxCorrupted     float64
	DemuxDiscontinuity float64
	TimeRemaining      float64 // seconds left in the current clip (length - playhead)
	Progress           float64 // 0..1 fraction of the current clip played
}

// VLCPlayerStats exposes the libvlc playback stats. Call Update on every
// poll tick with a fresh snapshot.
var VLCPlayerStats = vlcPlayerStatsIface{
	inputBitRate:       vlcInputBitRate,
	demuxBitRate:       vlcDemuxBitRate,
	displayedFPS:       vlcDisplayedFPS,
	decodedVideo:       vlcDecodedVideo,
	displayedPictures:  vlcDisplayedPictures,
	lostPictures:       vlcLostPictures,
	demuxCorrupted:     vlcDemuxCorrupted,
	demuxDiscontinuity: vlcDemuxDiscontinuity,
	timeRemaining:      vlcTimeRemaining,
	progress:           vlcProgress,
}

type vlcPlayerStatsIface struct {
	inputBitRate       metric.Float64Gauge
	demuxBitRate       metric.Float64Gauge
	displayedFPS       metric.Float64Gauge
	decodedVideo       metric.Float64Gauge
	displayedPictures  metric.Float64Gauge
	lostPictures       metric.Float64Gauge
	demuxCorrupted     metric.Float64Gauge
	demuxDiscontinuity metric.Float64Gauge
	timeRemaining      metric.Float64Gauge
	progress           metric.Float64Gauge
}

func (v vlcPlayerStatsIface) Update(s VLCPlayerStatsSnapshot) {
	ctx := context.Background()
	v.inputBitRate.Record(ctx, s.InputBitRate, platformAttr)
	v.demuxBitRate.Record(ctx, s.DemuxBitRate, platformAttr)
	v.displayedFPS.Record(ctx, s.DisplayedFPS, platformAttr)
	v.decodedVideo.Record(ctx, s.DecodedVideo, platformAttr)
	v.displayedPictures.Record(ctx, s.DisplayedPictures, platformAttr)
	v.lostPictures.Record(ctx, s.LostPictures, platformAttr)
	v.demuxCorrupted.Record(ctx, s.DemuxCorrupted, platformAttr)
	v.demuxDiscontinuity.Record(ctx, s.DemuxDiscontinuity, platformAttr)
	v.timeRemaining.Record(ctx, s.TimeRemaining, platformAttr)
	v.progress.Record(ctx, s.Progress, platformAttr)
}

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
	OutputBytes      float64
	OutputDurationMS float64
	OutputCongestion float64
	Reconnecting     bool
	SkippedFrames    float64
	TotalFrames      float64
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

type obsStreamingIface struct{ g metric.Int64Gauge }

func (o obsStreamingIface) Set(active bool) {
	v := int64(0)
	if active {
		v = 1
	}
	o.g.Record(context.Background(), v, platformAttr)
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
	o.activeFPS.Record(ctx, s.ActiveFPS, platformAttr)
	o.averageFrameRender.Record(ctx, s.AverageFrameRenderTime, platformAttr)
	o.cpuUsage.Record(ctx, s.CPUUsage, platformAttr)
	o.memoryUsage.Record(ctx, s.MemoryUsage, platformAttr)
	o.renderSkippedFrames.Record(ctx, s.RenderSkippedFrames, platformAttr)
	o.renderTotalFrames.Record(ctx, s.RenderTotalFrames, platformAttr)
	o.outputSkippedFrames.Record(ctx, s.OutputSkippedFrames, platformAttr)
	o.outputTotalFrames.Record(ctx, s.OutputTotalFrames, platformAttr)
}

func (o obsStatsIface) UpdateStream(s OBSStreamSnapshot) {
	ctx := context.Background()
	o.streamBytes.Record(ctx, s.OutputBytes, platformAttr)
	o.streamDuration.Record(ctx, s.OutputDurationMS, platformAttr)
	o.streamCongestion.Record(ctx, s.OutputCongestion, platformAttr)
	v := int64(0)
	if s.Reconnecting {
		v = 1
	}
	o.streamReconnecting.Record(ctx, v, platformAttr)
	o.streamSkipped.Record(ctx, s.SkippedFrames, platformAttr)
	o.streamTotal.Record(ctx, s.TotalFrames, platformAttr)
}
