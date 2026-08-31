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
// Every series here carries the platform. The watchdog runs on every platform —
// any platform can select any bed — so one instance per platform writes each of
// these, and without the label they differ only by pod name. A rule reading
// them then has no platform to group by: max() over the lot collapses to one
// number that only goes silent when every platform does at once, which is how
// prod TikTok ran silent for eight minutes on 2026-07-29 with the metrics
// collecting and nothing firing.
var (
	obsBackgroundAudioLevelDB = mustFloat64Gauge("obs_background_audio_level_db",
		"Peak output level (dBFS, floored at -60) of the platform's background-audio source. Silence (≈ -60) for a sustained window is the 'no audio on stream' alert signal.")
	obsBackgroundAudioPlaying = mustGauge("obs_background_audio_playing",
		"1 when the platform's background-audio source's OBS media state is PLAYING, 0 when it is ended/stopped/errored/none.")
	obsBackgroundAudioOnFallback = mustGauge("obs_background_audio_on_fallback",
		"1 when the audio-fallback watchdog has swapped the platform's background bed to the local Car Hum file because SomaFM was unreachable, 0 when on the normal SomaFM source.")
	somafmReachable = mustGauge("somafm_reachable",
		"1 when the SomaFM edge served stream bytes on the watchdog's last probe, 0 when it timed out or returned no data. Gates the swap back from the local fallback, and reported only while a bot is waiting on that gate: absent in steady state, so read a gap as 'nothing needed the edge', not as 'the edge is down'.")
	obsBackgroundAudioSwaps = mustCounter("obs_background_audio_swaps_total",
		"Total background-audio source swaps performed by the audio-fallback watchdog, labeled by direction (to_fallback|to_somafm). Any to_fallback increment means SomaFM dropped in prod.")
	backgroundAudioBed = mustGauge("tripbot_background_audio_bed",
		"1 for the background-audio bed on air, 0 for every other bed, labeled by bed (somafm|carhum|album) and platform. Exactly one bed per platform reads 1 at a time, so this is the 'what is the stream playing' series.")
	backgroundAudioAlbumTracks = mustGauge("tripbot_background_audio_album_tracks",
		"Number of tracks in the album bed's play order, labeled by platform. 0 while the album bed is on air means nothing advances when the current track ends: the stream falls silent with no error, no log line, and OBS still reporting a healthy source. Alert on album=1 AND tracks=0.")
)

// OBSBackgroundAudio exposes the background-audio gauges + swap counter.
// The audio-fallback watchdog records the gauges every tick and increments the
// swap counter on each source change.
// The bed store records SetBed / SetAlbumTracks whenever the live bed changes
// or is re-read off OBS.
var OBSBackgroundAudio = obsBackgroundAudioMetrics{
	levelDB:     obsBackgroundAudioLevelDB,
	playing:     obsBackgroundAudioPlaying,
	onFallback:  obsBackgroundAudioOnFallback,
	reachable:   somafmReachable,
	swaps:       obsBackgroundAudioSwaps,
	bed:         backgroundAudioBed,
	albumTracks: backgroundAudioAlbumTracks,
}

type obsBackgroundAudioMetrics struct {
	levelDB     metric.Float64Gauge
	playing     metric.Int64Gauge
	onFallback  metric.Int64Gauge
	reachable   metric.Int64Gauge
	swaps       metric.Int64Counter
	bed         metric.Int64Gauge
	albumTracks metric.Int64Gauge
}

// SetLevelDB records the latest peak output level (already floored at -60).
func (o obsBackgroundAudioMetrics) SetLevelDB(platform string, db float64) {
	o.levelDB.Record(context.Background(), db, platformAttr(platform))
}

// SetPlaying records whether OBS reports the source's media state as PLAYING.
func (o obsBackgroundAudioMetrics) SetPlaying(platform string, playing bool) {
	o.playing.Record(context.Background(), b2i(playing), platformAttr(platform))
}

// SetOnFallback records whether the watchdog has the source on the local bed.
func (o obsBackgroundAudioMetrics) SetOnFallback(platform string, on bool) {
	o.onFallback.Record(context.Background(), b2i(on), platformAttr(platform))
}

// SetSomaFMReachable records the last SomaFM edge probe result.
func (o obsBackgroundAudioMetrics) SetSomaFMReachable(platform string, reachable bool) {
	o.reachable.Record(context.Background(), b2i(reachable), platformAttr(platform))
}

// SetBed records whether a bed is the one on air for a platform. The caller
// writes every bed it knows about on each change — the live one and the rest —
// rather than only the winner, so no series is left reading 1 for a bed that
// stopped playing. Bed switches are rare, so the extra writes are free.
func (o obsBackgroundAudioMetrics) SetBed(platform, bed string, live bool) {
	o.bed.Record(context.Background(), b2i(live),
		metric.WithAttributes(attribute.String("bed", bed)), platformAttr(platform))
}

// SetAlbumTracks records how many tracks the album play order holds. 0 is the
// interesting value: on the album bed it means the next track-ended event has
// nowhere to advance to.
func (o obsBackgroundAudioMetrics) SetAlbumTracks(platform string, n int) {
	o.albumTracks.Record(context.Background(), int64(n), platformAttr(platform))
}

// IncSwap counts a source swap; direction is "to_fallback" or "to_somafm".
func (o obsBackgroundAudioMetrics) IncSwap(platform, direction string) {
	o.swaps.Add(context.Background(), 1,
		metric.WithAttributes(attribute.String("direction", direction)), platformAttr(platform))
}
