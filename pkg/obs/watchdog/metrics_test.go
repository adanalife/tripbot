package watchdog

import (
	"context"
	"maps"
	"sync"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// The restart counter is the one signal that says "recovery is running." It has
// to move on a recovery that fails, because a recovery that keeps failing is
// the worse outage: through 9h41m on 2026-08-05 the watchdog attempted a
// restart every 60s, failed every time, and the counter — incremented only past
// the error check — never moved, so the panel over it read a flat zero for the
// whole incident while the stream was dark.
//
// Reads the real datapoints through an SDK reader rather than trusting the call
// site by inspection, so re-introducing the ordering bug fails here.
func TestWatchSilentDisconnect_CountsFailedRestarts(t *testing.T) {
	// Three misses fire a restart; the staged step fails it. A cooldown longer
	// than the run keeps the count at exactly one attempt.
	got := restartsDuring(t, func() {
		runFixtureOn(t, "tiktok", []step{missNoFix, missNoFix, missNoFix}, 3, longCooldown)
	})
	if want := map[string]int64{"tiktok/failed": 1}; !maps.Equal(got, want) {
		t.Errorf("restart datapoints = %v, want %v", got, want)
	}
}

// The success path has to stay distinguishable from the failure path, or the
// result attribute buys nothing over the bare count it replaced.
func TestWatchSilentDisconnect_CountsSuccessfulRestarts(t *testing.T) {
	got := restartsDuring(t, func() {
		runFixtureOn(t, "youtube", []step{miss, miss, miss}, 3, longCooldown)
	})
	if want := map[string]int64{"youtube/ok": 1}; !maps.Equal(got, want) {
		t.Errorf("restart datapoints = %v, want %v", got, want)
	}
}

// longCooldown outlives any scripted run, so these tests measure one attempt
// rather than however many the cooldown would have let through.
const longCooldown = watchInterval * 1000

// restartsDuring reports the restart datapoints the counter gained while body
// ran, keyed by "<service.platform>/<result>".
//
// A delta rather than an absolute: the counter is process-wide and every test
// in this package feeds the same one.
func restartsDuring(t *testing.T, body func()) map[string]int64 {
	t.Helper()
	reader := meterReader()
	before := restartsByResult(t, reader)
	body()
	got := restartsByResult(t, reader)
	for k, v := range before {
		if got[k] -= v; got[k] == 0 {
			delete(got, k)
		}
	}
	return got
}

// meterReader points the global meter provider at a manual reader, once.
//
// pkg/instrumentation builds its instruments at package init from otel.Meter,
// which returns a global-delegating meter, so installing a provider here makes
// those already-constructed instruments record into this reader. OTel honours
// exactly one SetMeterProvider per process — a second install is silently
// ignored and its reader would collect nothing — hence the Once and the shared
// reader.
var meterReader = sync.OnceValue(func() *metric.ManualReader {
	reader := metric.NewManualReader()
	otel.SetMeterProvider(metric.NewMeterProvider(metric.WithReader(reader)))
	return reader
})

// restartsByResult collects the restart counter's current datapoints, keyed by
// "<service.platform>/<result>".
func restartsByResult(t *testing.T, reader *metric.ManualReader) map[string]int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	got := map[string]int64{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "tripbot_obs_silent_disconnect_restarts_total" {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("%s is %T, want an int64 Sum", m.Name, m.Data)
			}
			for _, dp := range sum.DataPoints {
				result, _ := dp.Attributes.Value("result")
				platform, ok := dp.Attributes.Value("service.platform")
				if !ok {
					t.Errorf("datapoint %v has no service.platform; the per-platform legs would share one series", dp.Attributes)
				}
				got[platform.AsString()+"/"+result.AsString()] += dp.Value
			}
		}
	}
	return got
}

// The stood-down state is a gauge the alert reads directly, so it has to read 1
// for exactly as long as the watchdog is stood down and 0 again once the
// channel is back. A counter would go quiet within a window of the moment it
// happened, and "still stood down 17 hours later" is the reading that matters.
func TestWatchSilentDisconnect_ExhaustedGaugeTracksStandDown(t *testing.T) {
	reader := meterReader()
	runFixtureOn(t, "youtube", repeat(miss, 12), 3, 3*watchInterval)
	if got := exhaustedByPlatform(t, reader)["youtube"]; got != 1 {
		t.Errorf("exhausted gauge after the cap = %d, want 1", got)
	}
	runFixtureOn(t, "youtube", append(repeat(miss, 12), healthy, healthy, healthy), 3, 3*watchInterval)
	if got := exhaustedByPlatform(t, reader)["youtube"]; got != 0 {
		t.Errorf("exhausted gauge after the channel came back = %d, want 0", got)
	}
}

// exhaustedByPlatform reads the stood-down gauge's current value per
// service.platform.
func exhaustedByPlatform(t *testing.T, reader *metric.ManualReader) map[string]int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	got := map[string]int64{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "tripbot_obs_recovery_exhausted" {
				continue
			}
			g, ok := m.Data.(metricdata.Gauge[int64])
			if !ok {
				t.Fatalf("%s is %T, want an int64 Gauge", m.Name, m.Data)
			}
			for _, dp := range g.DataPoints {
				platform, _ := dp.Attributes.Value("service.platform")
				got[platform.AsString()] = dp.Value
			}
		}
	}
	return got
}
