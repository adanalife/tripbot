package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/httpmw"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	sentrynegroni "github.com/getsentry/sentry-go/negroni"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	metrics "github.com/slok/go-http-metrics/metrics/prometheus"
	"github.com/slok/go-http-metrics/middleware"
	negronimiddleware "github.com/slok/go-http-metrics/middleware/negroni"
	"github.com/unrolled/secure"
	"github.com/urfave/negroni/v3"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

var server *http.Server

// shutdownTimeout is how long Shutdown waits for in-flight requests to
// finish before forcing connections closed. 15s is the typical sweet spot:
// long enough that healthy requests complete, short enough that a stuck
// handler doesn't block process exit indefinitely.
const shutdownTimeout = 15 * time.Second

// Start starts the web server. When ctx is canceled (e.g. SIGINT/SIGTERM
// via signal.NotifyContext) the server stops accepting new connections and
// waits up to shutdownTimeout for in-flight requests to complete before
// returning.
func Start(ctx context.Context) {
	slog.InfoContext(ctx, "starting web server", "port", c.Conf.TripbotServerPort)

	r := mux.NewRouter()

	// healthcheck endpoints
	hp := r.PathPrefix("/health").Methods("GET", "HEAD").Subrouter()
	hp.Handle("/live", tagged("/health/live", httpmw.LivenessHandler()))
	// /ready runs no checks: tripbot's HTTP surface (admin panel, /auth/init,
	// /auth/callback, /metrics) doesn't depend on the Twitch connection, so the
	// pod must stay routable even when the bot is offline. Chat-connection is
	// surfaced via the admin panel + the tripbot_twitch_connected gauge.
	hp.Handle("/ready", tagged("/health/ready", httpmw.ReadinessHandler()))

	// version endpoint — returns build metadata as JSON
	r.Handle("/version", tagged("/version", versionHandler)).Methods("GET", "HEAD")

	// auth endpoints
	auth := r.PathPrefix("/auth").Methods("GET").Subrouter()
	auth.Handle("/init", tagged("/auth/init", authInitHandler))
	auth.Handle("/callback", tagged("/auth/callback", authCallbackHandler))

	// static assets
	r.Handle("/favicon.ico", tagged("/favicon.ico", faviconHandler)).Methods("GET")

	// prometheus metrics endpoint
	r.Path("/metrics").Handler(tagged("/metrics", promhttp.Handler().ServeHTTP))

	// admin panel (status overview + links) on the root path
	r.Handle("/", tagged("/", adminHandler)).Methods("GET", "HEAD")

	// live console: SSE stream the panel subscribes to (GET, long-lived) +
	// the vendored htmx assets it loads. The /admin POST subrouter below is
	// POST-only, so the GET stream registers on r directly.
	r.Handle("/admin/events", tagged("/admin/events", eventsHandler)).Methods("GET")
	r.Handle("/admin/user/{username}", tagged("/admin/user/{username}", userProfileHandler)).Methods("GET")
	r.Handle("/admin/map/corpus", tagged("/admin/map/corpus", mapCorpusHandler)).Methods("GET")
	r.PathPrefix("/static/").Handler(staticHandler())

	// admin actions — tailnet-only by virtue of where the Ingress is
	// exposed; no app-layer auth gate (see CLAUDE.md / vault decisions).
	admin := r.PathPrefix("/admin").Methods("POST").Subrouter()
	admin.Handle("/obs/stream/{action}", tagged("/admin/obs/stream/{action}", obsStreamActionHandler))
	admin.Handle("/shutdown", tagged("/admin/shutdown", httpmw.ShutdownHandler()))
	admin.Handle("/restart/{service}", tagged("/admin/restart/{service}", restartActionHandler))

	// catch everything else
	r.NotFoundHandler = tagged("/", catchAllHandler)

	if c.Conf.Verbose {
		helpers.PrintAllRoutes(r)
	}

	// negroni.New + explicit middleware so we can swap negroni's stdlib
	// logger for an slog-based one — see pkg/httpmw.SlogLogger. The static
	// middleware from negroni.Classic is dropped (no public/ directory).
	app := negroni.New(
		httpmw.NewRecovery(func(any) { instrumentation.HTTPPanics.Inc(c.Conf.ServerType) }),
		httpmw.NewSlogLogger(),
	)

	// attach http-metrics (prometheus) middleware
	metricsMw := middleware.New(middleware.Config{
		Recorder: metrics.NewRecorder(metrics.Config{}),
		Service:  c.Conf.ServerType,
	})
	app.Use(negronimiddleware.Handler("", metricsMw))

	// attach security middleware
	secureMw := secure.New(secure.Options{
		FrameDeny:     true,
		IsDevelopment: c.Conf.IsDevelopment(),
	})
	app.Use(negroni.HandlerFunc(secureMw.HandlerFuncWithNext))

	// attach Sentry middleware (for reporting exceptions)
	app.Use(sentrynegroni.New(sentrynegroni.Options{}))

	// attaching routes to handler happens last
	app.UseHandler(r)

	srv := &http.Server{
		Addr: fmt.Sprintf("0.0.0.0:%s", c.Conf.TripbotServerPort),
		// WriteTimeout is 0 (disabled) because the admin panel's live console
		// streams Server-Sent Events on /admin/events — a long-lived response a
		// fixed write deadline would sever. The Go-idiomatic per-request
		// http.ResponseController.SetWriteDeadline doesn't reach the underlying
		// writer through the negroni + otelhttp (httpsnoop) HTTP/2 wrapper chain
		// ("feature not supported"), so disabling it server-wide is the reliable
		// fix. Slowloris protection is preserved by ReadHeaderTimeout (the header
		// read is the attack vector WriteTimeout never really guarded anyway).
		ReadTimeout:       time.Second * 15,
		ReadHeaderTimeout: time.Second * 15,
		WriteTimeout:      0,
		IdleTimeout:       time.Second * 60,
		MaxHeaderBytes:    1 << 20, // 1 MB
		Handler:           otelhttp.NewHandler(app, c.Conf.ServerType),
	}

	// Run ListenAndServe in a goroutine so we can block on ctx.Done() and
	// trigger a graceful shutdown when a signal arrives. ErrServerClosed is
	// the expected return after Shutdown is called and is not an error.
	serverErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case err := <-serverErr:
		if err != nil {
			terrors.FatalContext(ctx, err, "couldn't start server")
		}
	case <-ctx.Done():
		slog.InfoContext(ctx, "shutting down web server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.ErrorContext(shutdownCtx, "error during web server shutdown", "err", err)
		}
	}
}

// tagged wraps a HandlerFunc so the http.route attribute is set on metrics
// (via otelhttp.Labeler) and traces (via the active span), and overrides
// the span name with the route template so spans group by route in Tempo
// instead of all collapsing under the operation name passed to
// otelhttp.NewHandler. Negroni doesn't surface the underlying mux route
// template to the otelhttp middleware, so each registration declares it.
func tagged(route string, h http.HandlerFunc) http.Handler {
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
