package instrumentation

import (
	"encoding/json"
	"slices"
	"strings"
)

// seriesNames collects the Prometheus series every must* helper produces, so
// the generated metrics.json lists exactly what this package emits. It is
// filled during package-var initialization and only read afterwards.
var seriesNames []string

// register records the Prometheus names an OTel instrument exports under: the
// OTLP→Prometheus translation appends _total to a counter that lacks it and
// splits a histogram into _bucket/_count/_sum; a gauge keeps its name.
func register(name, kind string) {
	switch kind {
	case "counter":
		if !strings.HasSuffix(name, "_total") {
			name += "_total"
		}
		seriesNames = append(seriesNames, name)
	case "histogram":
		seriesNames = append(seriesNames, name+"_bucket", name+"_count", name+"_sum")
	default:
		seriesNames = append(seriesNames, name)
	}
}

const metricsComment = "Generated from pkg/instrumentation's must* registry via `go generate ./pkg/contract` — do not hand-edit. Every Prometheus series name tripbot emits, as stored after the OTLP translation (counters end _total, histograms split into _bucket/_count/_sum). infra syncs it so an alert rule naming a tripbot metric that no longer exists fails pre-merge. nats_connected (pkg/natsclient) is registered outside this package and not listed."

// MarshalMetricNames renders the sorted series-name list as metrics.json.
func MarshalMetricNames() ([]byte, error) {
	names := slices.Clone(seriesNames)
	slices.Sort(names)
	out, err := json.MarshalIndent(struct {
		Comment string   `json:"_comment"`
		Metrics []string `json:"metrics"`
	}{metricsComment, slices.Compact(names)}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}
