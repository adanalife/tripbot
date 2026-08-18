package instrumentation

import (
	"context"
	"maps"
	"testing"

	"go.opentelemetry.io/otel"
	metricsdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// CurrentState.Set must track exactly one active state at a time: a blank
// abbrev normalizes to "unknown", a transition advances prev to the new
// state, and a repeated Set of the same state is a no-op. The OTel gauge
// values themselves aren't read back here (no reader is wired in unit tests);
// the prev bookkeeping is the load-bearing "clear the old =1 series" logic, so
// that's what we assert.
func TestCurrentStateSet_TracksActiveState(t *testing.T) {
	s := &currentStateGauge{gauge: currentState}

	tests := []struct {
		name     string
		in       string
		wantPrev string
	}{
		{"first set", "MO", "MO"},
		{"transition", "KS", "KS"},
		{"repeat is no-op", "KS", "KS"},
		{"blank normalizes to unknown", "", "unknown"},
		{"recover from unknown", "CO", "CO"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s.Set(tt.in, "twitch")
			if s.prev != tt.wantPrev {
				t.Errorf("after Set(%q), prev = %q, want %q", tt.in, s.prev, tt.wantPrev)
			}
		})
	}
}

// The package-level CurrentState must be usable as a no-config no-op (the
// default OTel meter swallows records), matching how every other recorder in
// this package behaves when no exporter is configured.
func TestCurrentStateSet_DefaultIsSafe(t *testing.T) {
	CurrentState.Set("WY", "twitch")
	CurrentState.Set("", "")
}

// The events counter has to carry service.platform, not just event. Without it
// a dashboard can only break the rate down by pod instance, and the
// twitch-chat-activity panels' service_platform selector matches nothing —
// which reads as "no events" rather than as a missing label.
//
// Reads the real datapoints through an SDK reader rather than trusting the call
// site by inspection, so dropping the attribute fails here.
func TestEventsInc_StampsEventAndPlatform(t *testing.T) {
	reader := metricsdk.NewManualReader()
	otel.SetMeterProvider(metricsdk.NewMeterProvider(metricsdk.WithReader(reader)))

	Events.Inc("state_crossing", "youtube")
	Events.Inc("login", "twitch")

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}

	got := map[string]int64{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "tripbot_events_total" {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("%s is %T, want an int64 Sum", m.Name, m.Data)
			}
			for _, dp := range sum.DataPoints {
				event, _ := dp.Attributes.Value("event")
				platform, ok := dp.Attributes.Value("service.platform")
				if !ok {
					t.Errorf("datapoint %v has no service.platform", dp.Attributes)
				}
				got[event.AsString()+"/"+platform.AsString()] += dp.Value
			}
		}
	}

	want := map[string]int64{"state_crossing/youtube": 1, "login/twitch": 1}
	if !maps.Equal(got, want) {
		t.Errorf("events datapoints = %v, want %v", got, want)
	}
}
