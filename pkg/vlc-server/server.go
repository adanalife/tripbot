package vlcServer

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/vlc-server"
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
	slog.InfoContext(ctx, "starting VLC web server", "bind", c.Conf.VlcServerBindAddress)

	r := mux.NewRouter()

	// healthcheck endpoints
	//TODO: handle HEAD requests here too
	hp := r.PathPrefix("/health").Methods("GET", "HEAD").Subrouter()
	hp.Handle("/", tagged("/health/", healthHandler))
	hp.Handle("/live", tagged("/health/live", healthHandler))
	hp.Handle("/ready", tagged("/health/ready", healthHandler))

	// version endpoint — returns build metadata as JSON
	r.Handle("/version", tagged("/version", versionHandler)).Methods("GET", "HEAD")

	// vlc endpoints
	vlc := r.PathPrefix("/vlc").Methods("GET").Subrouter()
	vlc.Handle("/current", tagged("/vlc/current", vlcCurrentHandler))
	vlc.Handle("/play/{video}", tagged("/vlc/play/{video}", vlcPlayHandler))
	vlc.Handle("/random", tagged("/vlc/random", vlcRandomHandler))
	vlc.Handle("/back", tagged("/vlc/back", vlcBackHandler))
	vlc.Handle("/back/{n}", tagged("/vlc/back/{n}", vlcBackHandler))
	vlc.Handle("/skip", tagged("/vlc/skip", vlcSkipHandler))
	vlc.Handle("/skip/{n}", tagged("/vlc/skip/{n}", vlcSkipHandler))

	// onscreen endpoints now live in cmd/onscreens-server (separate binary,
	// separate port). vlc-server no longer serves /onscreens/* — clients
	// (chatbot, OBS browser sources) hit ONSCREENS_SERVER_HOST directly.

	// prometheus metrics endpoint
	r.Path("/metrics").Handler(tagged("/metrics", promhttp.Handler().ServeHTTP))

	// static assets
	r.Handle("/favicon.ico", tagged("/favicon.ico", faviconHandler)).Methods("GET")

	// catch everything else
	r.Handle("/", tagged("/", catchAllHandler))

	if c.Conf.Verbose {
		helpers.PrintAllRoutes(r)
	}

	// negroni.New + explicit middleware so we can swap negroni's stdlib
	// logger for an slog-based one — see pkg/httpmw.SlogLogger. The static
	// middleware from negroni.Classic is dropped (no public/ directory).
	//TODO: consider adding HTMLPanicFormatter
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
		Addr: c.Conf.VlcServerBindAddress,
		// Good practice to set timeouts to avoid Slowloris attacks.
		WriteTimeout:   time.Second * 15,
		ReadTimeout:    time.Second * 15,
		IdleTimeout:    time.Second * 60,
		MaxHeaderBytes: 1 << 20, // 1 MB
		Handler:        otelhttp.NewHandler(app, c.Conf.ServerType),
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
		slog.InfoContext(ctx, "shutting down VLC web server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.ErrorContext(shutdownCtx, "error during VLC web server shutdown", "err", err)
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
