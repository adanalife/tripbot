package instrumentation

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Background-audio metrics for the Twitch stream's music bed ("Groove Salad
// Classic", a SomaFM ffmpeg_source). The 2026-06-23 outage showed the source
// can go silent in three ways with no self-heal — an EOF-wedge where OBS stops
// retrying, single-edge jitter, and a full SomaFM outage. These series are the
// alert signal and the audit trail for the audio-fallback watchdog
// (pkg/obs/audiowatchdog), which swaps the source to a local license-clean bed
// when SomaFM is down and swaps back when it recovers.
//
// Twitch-scoped: SomaFM is only the Twitch background bed (YouTube uses the
// local Car Hum drone, which can't drop), so these carry no platform label.
var (
	obsBackgroundAudioLevelDB = mustFloat64Gauge("obs_background_audio_level_db",
		"Peak output level (dBFS, floored at -60) of the Twitch background-audio source. Silence (≈ -60) for a sustained window is the 'no audio on stream' alert signal.")
	obsBackgroundAudioPlaying = mustGauge("obs_background_audio_playing",
		"1 when the Twitch background-audio source's OBS media state is PLAYING, 0 when it is ended/stopped/errored/none.")
	obsBackgroundAudioOnFallback = mustGauge("obs_background_audio_on_fallback",
		"1 when the audio-fallback watchdog has swapped the Twitch background bed to the local Car Hum file because SomaFM was unreachable, 0 when on the normal SomaFM source.")
	somafmReachable = mustGauge("somafm_reachable",
		"1 when the SomaFM edge served stream bytes on the watchdog's last probe, 0 when it timed out or returned no data. Gates the swap back from the local fallback.")
	obsBackgroundAudioSwaps = mustCounter("obs_background_audio_swaps_total",
		"Total background-audio source swaps performed by the audio-fallback watchdog, labeled by direction (to_fallback|to_somafm). Any to_fallback increment means SomaFM dropped in prod.")
)

// OBSBackgroundAudio exposes the Twitch background-audio gauges + swap counter.
// The audio-fallback watchdog records the gauges every tick and increments the
// swap counter on each source change.
var OBSBackgroundAudio = obsBackgroundAudioIface{
	levelDB:    obsBackgroundAudioLevelDB,
	playing:    obsBackgroundAudioPlaying,
	onFallback: obsBackgroundAudioOnFallback,
	reachable:  somafmReachable,
	swaps:      obsBackgroundAudioSwaps,
}

type obsBackgroundAudioIface struct {
	levelDB    metric.Float64Gauge
	playing    metric.Int64Gauge
	onFallback metric.Int64Gauge
	reachable  metric.Int64Gauge
	swaps      metric.Int64Counter
}

// SetLevelDB records the latest peak output level (already floored at -60).
func (o obsBackgroundAudioIface) SetLevelDB(db float64) {
	o.levelDB.Record(context.Background(), db)
}

// SetPlaying records whether OBS reports the source's media state as PLAYING.
func (o obsBackgroundAudioIface) SetPlaying(playing bool) {
	o.playing.Record(context.Background(), b2i(playing))
}

// SetOnFallback records whether the watchdog has the source on the local bed.
func (o obsBackgroundAudioIface) SetOnFallback(on bool) {
	o.onFallback.Record(context.Background(), b2i(on))
}

// SetSomaFMReachable records the last SomaFM edge probe result.
func (o obsBackgroundAudioIface) SetSomaFMReachable(reachable bool) {
	o.reachable.Record(context.Background(), b2i(reachable))
}

// IncSwap counts a source swap; direction is "to_fallback" or "to_somafm".
func (o obsBackgroundAudioIface) IncSwap(direction string) {
	o.swaps.Add(context.Background(), 1, metric.WithAttributes(attribute.String("direction", direction)))
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
