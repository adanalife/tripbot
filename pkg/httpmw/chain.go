package httpmw

import (
	metrics "github.com/slok/go-http-metrics/metrics/prometheus"
	"github.com/slok/go-http-metrics/middleware"
	negronimiddleware "github.com/slok/go-http-metrics/middleware/negroni"
	"github.com/unrolled/secure"
	"github.com/urfave/negroni/v3"
)

// NewChain builds the middleware chain both HTTP servers share, outermost
// first: panic recovery, slog request logging, Prometheus HTTP metrics
// labeled with service, security headers, and Sentry. The caller attaches
// its router with UseHandler.
//
// negroni.New + explicit middleware swaps negroni's stdlib logger for an
// slog-based one (see SlogLogger). The static middleware from
// negroni.Classic is dropped (no public/ directory). onPanic runs on every
// recovered handler panic; isDevelopment relaxes the security middleware.
func NewChain(service string, isDevelopment bool, onPanic func(any)) *negroni.Negroni {
	app := negroni.New(
		NewRecovery(onPanic),
		NewSlogLogger(),
	)

	// attach http-metrics (prometheus) middleware
	metricsMw := middleware.New(middleware.Config{
		Recorder: metrics.NewRecorder(metrics.Config{}),
		Service:  service,
	})
	app.Use(negronimiddleware.Handler("", metricsMw))

	// attach security middleware
	secureMw := secure.New(secure.Options{
		FrameDeny:     true,
		IsDevelopment: isDevelopment,
	})
	app.Use(negroni.HandlerFunc(secureMw.HandlerFuncWithNext))

	// attach Sentry middleware (for reporting exceptions)
	app.Use(NewSentry())

	return app
}
