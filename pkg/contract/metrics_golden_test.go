package contract_test

import (
	_ "embed"
	"testing"

	"github.com/adanalife/tripbot/pkg/instrumentation"
)

//go:embed metrics.json
var committedMetrics []byte

// TestMetricsMatchesCommitted regenerates metrics.json from the live must*
// registry in pkg/instrumentation and asserts it is byte-identical to the
// committed file, so adding, renaming or removing a metric fails here until
// the file is regenerated — and infra's alert check sees the change.
func TestMetricsMatchesCommitted(t *testing.T) {
	got, err := instrumentation.MarshalMetricNames()
	if err != nil {
		t.Fatalf("MarshalMetricNames: %v", err)
	}
	if string(got) != string(committedMetrics) {
		t.Errorf("metrics.json is out of date with pkg/instrumentation's registry.\n"+
			"Run `go generate ./pkg/contract` and commit the result.\n\n"+
			"--- committed metrics.json ---\n%s\n--- generated ---\n%s",
			committedMetrics, got)
	}
}
