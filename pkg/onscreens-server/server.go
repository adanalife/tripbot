package onscreensServer

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/onscreens-server"
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

// Config bundles the runtime knobs cmd/onscreens-server passes into New.
// Build-time configuration (bind address, server type, etc.) still flows
// through the package-level config.Conf var imported as `c` — Config is
// only for the handful of values that vary per process invocation.
type Config struct {
	// Version is the build-time tag returned by /version. Typically set
	// from cmd/onscreens-server's `main.version` var, which is overridden
	// via `-ldflags "-X main.version=..."`.
	Version string
}

// Server owns the onscreen singletons and the HTTP listener that serves
// them. Construct via New; call Start to block on the HTTP listener.
type Server struct {
	Version string

	Flag         *Onscreen
	GPS          *Onscreen
	Leaderboard  *Onscreen
	LeftRotator  *Onscreen
	MiddleText   *Onscreen
	RightRotator *Onscreen
	Timewarp     *Onscreen

	http *http.Server
}

// New constructs a *Server with all seven onscreens initialised and
// their background loops (rotators, expiry sweepers) running. It does
// not bind any sockets — Start does that.
func New(cfg Config) *Server {
	version := cfg.Version
	if version == "" {
		version = "dev"
	}
	return &Server{
		Version:      version,
		Flag:         newFlagOnscreen(),
		GPS:          newGPSOnscreen(),
		Leaderboard:  newLeaderboardOnscreen(),
		LeftRotator:  newLeftRotator(),
		MiddleText:   newMiddleText(),
		RightRotator: newRightRotator(),
		Timewarp:     newTimewarp(),
	}
}

// Start brings up the HTTP listener serving all /onscreens/* routes plus
// the standard /health, /version, /metrics surface. It blocks until either
// ListenAndServe returns an error or ctx is canceled (typically by the
// process-level signal handler in cmd/onscreens-server). Callers should
// fatal on a non-nil error.
//
// When ctx is canceled Start returns nil; the caller is responsible for
// invoking Shutdown with its own deadline so in-flight requests get a
// chance to drain. The split keeps Start's return contract simple
// (listener-error vs. clean-stop) and lets the signal handler control
// the shutdown deadline.
func (s *Server) Start(ctx context.Context) error {
	slog.InfoContext(ctx, "starting onscreens-server web server", "bind", c.Conf.OnscreensServerBindAddress)

	// Attach NATS subscribers. No-op when the natsclient singleton is
	// nil (NATS_URL unset); HTTP remains the sole transport in that case.
	s.StartNATSSubscribers(ctx)

	// Restore the permanent middle-text overlay from its JetStream last-value
	// cache so a server restart doesn't blank whatever text was on screen.
	// Best-effort: a no-op without NATS / JetStream.
	s.RestoreMiddleText(ctx)

	r := mux.NewRouter()

	// healthcheck endpoints
	hp := r.PathPrefix("/health").Methods("GET", "HEAD").Subrouter()
	hp.Handle("/", tagged("/health/", healthHandler))
	hp.Handle("/live", tagged("/health/live", healthHandler))
	hp.Handle("/ready", tagged("/health/ready", healthHandler))

	// version endpoint — returns build metadata as JSON
	r.Handle("/version", tagged("/version", s.versionHandler)).Methods("GET", "HEAD")

	// onscreen endpoints — commands (middle / leaderboard / timewarp / gps /
	// flag) arrive over NATS now (see nats.go); only the browser-source feeds
	// remain on HTTP: state JSON, per-onscreen HTML pages, and image assets.
	osc := r.PathPrefix("/onscreens").Methods("GET").Subrouter()
	osc.Handle("/state.json", tagged("/onscreens/state.json", s.onscreensStateHandler))
	osc.Handle("/render/{name}", tagged("/onscreens/render/{name}", s.onscreensRenderHandler))
	osc.Handle("/asset/{name}", tagged("/onscreens/asset/{name}", s.onscreensAssetHandler))

	// admin actions — tailnet-only by virtue of where the Ingress is exposed;
	// no app-layer auth gate. /admin/shutdown is the admin panel's "restart
	// onscreens-server" surface; the shared handler SIGTERMs the process and
	// k8s restartPolicy: Always brings the pod back.
	admin := r.PathPrefix("/admin").Methods("POST").Subrouter()
	admin.Handle("/shutdown", tagged("/admin/shutdown", httpmw.ShutdownHandler()))

	// prometheus metrics endpoint
	r.Path("/metrics").Handler(tagged("/metrics", promhttp.Handler().ServeHTTP))

	// catch everything else
	r.Handle("/", tagged("/", catchAllHandler))

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

	metricsMw := middleware.New(middleware.Config{
		Recorder: metrics.NewRecorder(metrics.Config{}),
		Service:  c.Conf.ServerType,
	})
	app.Use(negronimiddleware.Handler("", metricsMw))

	secureMw := secure.New(secure.Options{
		FrameDeny:     true,
		IsDevelopment: c.Conf.IsDevelopment(),
	})
	app.Use(negroni.HandlerFunc(secureMw.HandlerFuncWithNext))

	app.Use(sentrynegroni.New(sentrynegroni.Options{}))

	app.UseHandler(r)

	s.http = &http.Server{
		Addr:           c.Conf.OnscreensServerBindAddress,
		WriteTimeout:   time.Second * 15,
		ReadTimeout:    time.Second * 15,
		IdleTimeout:    time.Second * 60,
		MaxHeaderBytes: 1 << 20, // 1 MB
		Handler:        otelhttp.NewHandler(app, c.Conf.ServerType),
	}

	// Run ListenAndServe in a goroutine so Start can block on ctx.Done()
	// and return cleanly when the signal handler cancels the context. The
	// caller calls Shutdown after Start returns to drain in-flight
	// requests with its chosen deadline. ErrServerClosed is the expected
	// return after Shutdown is called and is not surfaced as an error.
	serverErr := make(chan error, 1)
	go func() {
		if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		return nil
	}
}

// Shutdown gracefully stops the HTTP listener, allowing in-flight
// requests up to ctx's deadline to complete before closing connections.
// Returns the error from http.Server.Shutdown so callers can log or act
// on a timeout. Safe to call once Start has returned (or concurrently
// while Start is blocked on ctx.Done()); calling on a zero-value Server
// (Start never ran) is a no-op.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.http == nil {
		return nil
	}
	return s.http.Shutdown(ctx)
}

// versionHandler returns build metadata as JSON. The tag comes from the
// build-time ldflag (threaded through Config{Version} into the Server);
// sha + built_at are read from the binary's embedded VCS info (Go's
// automatic -buildvcs). started_at is when the process began so callers
// can derive uptime themselves.
func (s *Server) versionHandler(w http.ResponseWriter, r *http.Request) {
	resp := struct {
		Tag       string `json:"tag"`
		Sha       string `json:"sha"`
		BuiltAt   string `json:"built_at"`
		StartedAt string `json:"started_at"`
	}{Tag: s.Version, StartedAt: startedAt.UTC().Format(time.RFC3339)}

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, set := range info.Settings {
			switch set.Key {
			case "vcs.revision":
				resp.Sha = set.Value
			case "vcs.time":
				resp.BuiltAt = set.Value
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.ErrorContext(r.Context(), "couldn't encode version response", "err", err)
	}
}

// startedAt marks process start so /version can report uptime. Set at
// package load; close enough to process start for a human-readable "up Xh".
var startedAt = time.Now()

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
