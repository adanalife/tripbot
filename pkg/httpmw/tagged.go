package httpmw

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// Tagged wraps a HandlerFunc so the http.route attribute is set on metrics
// (via otelhttp.Labeler) and traces (via the active span), and overrides
// the span name with the route template so spans group by route in Tempo
// instead of all collapsing under the operation name passed to
// otelhttp.NewHandler. Negroni doesn't surface the underlying mux route
// template to the otelhttp middleware, so each registration declares it.
func Tagged(route string, h http.HandlerFunc) http.Handler {
	attr := semconv.HTTPRoute(route)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		if labeler, ok := otelhttp.LabelerFromContext(ctx); ok {
			labeler.Add(attr)
		}
		span := trace.SpanFromContext(ctx)
		span.SetAttributes(attr)
		span.SetName(route)
		h(w, req)
	})
}
