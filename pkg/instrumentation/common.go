package instrumentation

import (
	"go.opentelemetry.io/otel/metric"
)

// HttpDuration tracks HTTP request durations. Reserved for future use —
// the active request-duration metric is supplied by slok/go-http-metrics
// in pkg/server's negroni stack.
var HttpDuration = mustHistogram("vlc_server_http_duration", "Duration of HTTP requests.", "s")

func mustHistogram(name, desc, unit string) metric.Float64Histogram {
	h, err := meter.Float64Histogram(name,
		metric.WithDescription(desc),
		metric.WithUnit(unit),
	)
	if err != nil {
		panic(err)
	}
	return h
}
