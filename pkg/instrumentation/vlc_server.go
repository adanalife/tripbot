package instrumentation

import (
	"context"

	"go.opentelemetry.io/otel/metric"
)

var (
	vlcInputBitRate        = mustFloat64Gauge("vlc_player_input_bitrate", "libvlc input bitrate (libvlc-native units, ~bytes/µs)")
	vlcDemuxBitRate        = mustFloat64Gauge("vlc_player_demux_bitrate", "libvlc demux bitrate (libvlc-native units, ~bytes/µs)")
	vlcDisplayedFPS        = mustFloat64Gauge("vlc_player_displayed_fps", "Derived frames-per-second: delta of displayed pictures over the poll interval")
	vlcDecodedVideo        = mustFloat64Gauge("vlc_player_decoded_video_frames", "Total decoded video blocks since the current Media started (resets on media change)")
	vlcDisplayedPictures   = mustFloat64Gauge("vlc_player_displayed_pictures", "Total displayed frames since the current Media started (resets on media change)")
	vlcLostPictures        = mustFloat64Gauge("vlc_player_lost_pictures", "Total lost (dropped) frames since the current Media started (resets on media change)")
	vlcDemuxCorrupted      = mustFloat64Gauge("vlc_player_demux_corrupted", "Demux corruptions discarded since the current Media started")
	vlcDemuxDiscontinuity  = mustFloat64Gauge("vlc_player_demux_discontinuity", "Demux discontinuities dropped since the current Media started")
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
}

func (v vlcPlayerStatsIface) Update(s VLCPlayerStatsSnapshot) {
	ctx := context.Background()
	v.inputBitRate.Record(ctx, s.InputBitRate)
	v.demuxBitRate.Record(ctx, s.DemuxBitRate)
	v.displayedFPS.Record(ctx, s.DisplayedFPS)
	v.decodedVideo.Record(ctx, s.DecodedVideo)
	v.displayedPictures.Record(ctx, s.DisplayedPictures)
	v.lostPictures.Record(ctx, s.LostPictures)
	v.demuxCorrupted.Record(ctx, s.DemuxCorrupted)
	v.demuxDiscontinuity.Record(ctx, s.DemuxDiscontinuity)
}
