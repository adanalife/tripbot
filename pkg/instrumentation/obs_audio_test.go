package instrumentation

import (
	"context"
	"maps"
	"testing"

	metricsdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// Built on their own provider rather than the package-level global: the global
// instruments delegate to the first provider set, so a test that shares them
// depends on which test ran first.
func testAudioMetrics(t *testing.T) (obsBackgroundAudioMetrics, *metricsdk.ManualReader) {
	t.Helper()
	reader := metricsdk.NewManualReader()
	meter := metricsdk.NewMeterProvider(metricsdk.WithReader(reader)).Meter("test")

	levelDB, err := meter.Float64Gauge("obs_background_audio_level_db")
	if err != nil {
		t.Fatalf("level gauge: %v", err)
	}
	playing, err := meter.Int64Gauge("obs_background_audio_playing")
	if err != nil {
		t.Fatalf("playing gauge: %v", err)
	}
	onFallback, err := meter.Int64Gauge("obs_background_audio_on_fallback")
	if err != nil {
		t.Fatalf("fallback gauge: %v", err)
	}
	reachable, err := meter.Int64Gauge("somafm_reachable")
	if err != nil {
		t.Fatalf("reachable gauge: %v", err)
	}
	swaps, err := meter.Int64Counter("obs_background_audio_swaps_total")
	if err != nil {
		t.Fatalf("swaps counter: %v", err)
	}
	return obsBackgroundAudioMetrics{
		levelDB:    levelDB,
		playing:    playing,
		onFallback: onFallback,
		reachable:  reachable,
		swaps:      swaps,
	}, reader
}

func collectAudio(t *testing.T, reader *metricsdk.ManualReader) []metricdata.Metrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	var out []metricdata.Metrics
	for _, sm := range rm.ScopeMetrics {
		out = append(out, sm.Metrics...)
	}
	return out
}

// The dead-air gauges have to carry service.platform. One watchdog runs per
// platform and they all write these, so without the label the series differ
// only by pod name and nothing downstream has a platform to group by: an alert
// on max() over the lot collapses to one number that reads silent only when
// every platform is silent at once. Prod TikTok ran silent for eight minutes on
// 2026-07-29 with these metrics collecting and the rule not firing.
//
// Reads the real datapoints through an SDK reader rather than trusting the call
// site by inspection, so dropping the attribute fails here.
func TestBackgroundAudioGauges_StampPlatform(t *testing.T) {
	audio, reader := testAudioMetrics(t)

	audio.SetPlaying("youtube", true)
	audio.SetPlaying("tiktok", false)
	audio.SetOnFallback("twitch", true)
	audio.SetSomaFMReachable("twitch", false)

	got := map[string]int64{}
	for _, m := range collectAudio(t, reader) {
		gauge, ok := m.Data.(metricdata.Gauge[int64])
		if !ok {
			continue
		}
		for _, dp := range gauge.DataPoints {
			platform, ok := dp.Attributes.Value("service.platform")
			if !ok {
				t.Errorf("%s datapoint %v has no service.platform", m.Name, dp.Attributes)
				continue
			}
			got[m.Name+"/"+platform.AsString()] = dp.Value
		}
	}

	want := map[string]int64{
		"obs_background_audio_playing/youtube":    1,
		"obs_background_audio_playing/tiktok":     0,
		"obs_background_audio_on_fallback/twitch": 1,
		"somafm_reachable/twitch":                 0,
	}
	if !maps.Equal(got, want) {
		t.Errorf("gauge datapoints = %v, want %v", got, want)
	}
}

// The level gauge is the alert's actual input, and a float, so it collects as a
// separate data shape from the ones above.
func TestBackgroundAudioLevel_StampsPlatform(t *testing.T) {
	audio, reader := testAudioMetrics(t)

	audio.SetLevelDB("youtube", -12.5)
	audio.SetLevelDB("tiktok", -60)

	got := map[string]float64{}
	for _, m := range collectAudio(t, reader) {
		if m.Name != "obs_background_audio_level_db" {
			continue
		}
		gauge, ok := m.Data.(metricdata.Gauge[float64])
		if !ok {
			t.Fatalf("%s is %T, want a float64 Gauge", m.Name, m.Data)
		}
		for _, dp := range gauge.DataPoints {
			platform, ok := dp.Attributes.Value("service.platform")
			if !ok {
				t.Fatalf("level datapoint %v has no service.platform", dp.Attributes)
			}
			got[platform.AsString()] = dp.Value
		}
	}

	want := map[string]float64{"youtube": -12.5, "tiktok": -60}
	if !maps.Equal(got, want) {
		t.Errorf("level datapoints = %v, want %v", got, want)
	}
}

// A swap is per-platform too: without the label a to_fallback on youtube and
// one on twitch add into one number, and "SomaFM dropped in prod" stops naming
// which stream it dropped on.
func TestBackgroundAudioSwaps_StampDirectionAndPlatform(t *testing.T) {
	audio, reader := testAudioMetrics(t)

	audio.IncSwap("twitch", "to_fallback")
	audio.IncSwap("youtube", "to_fallback")
	audio.IncSwap("twitch", "to_somafm")

	got := map[string]int64{}
	for _, m := range collectAudio(t, reader) {
		if m.Name != "obs_background_audio_swaps_total" {
			continue
		}
		sum, ok := m.Data.(metricdata.Sum[int64])
		if !ok {
			t.Fatalf("%s is %T, want an int64 Sum", m.Name, m.Data)
		}
		for _, dp := range sum.DataPoints {
			direction, _ := dp.Attributes.Value("direction")
			platform, ok := dp.Attributes.Value("service.platform")
			if !ok {
				t.Fatalf("swap datapoint %v has no service.platform", dp.Attributes)
			}
			got[platform.AsString()+"/"+direction.AsString()] += dp.Value
		}
	}

	want := map[string]int64{
		"twitch/to_fallback":  1,
		"youtube/to_fallback": 1,
		"twitch/to_somafm":    1,
	}
	if !maps.Equal(got, want) {
		t.Errorf("swap datapoints = %v, want %v", got, want)
	}
}
