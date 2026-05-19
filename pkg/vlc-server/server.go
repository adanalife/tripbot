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
	libvlc "github.com/adrg/libvlc-go/v3"
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

// Config is the construction-time configuration passed to New. It carries
// only the runtime knobs the binary needs to inject at startup; everything
// else flows in via the package-level c.Conf read from env.
type Config struct {
	// Version is the build-time version string, surfaced via /version.
	Version string
}

// Server is the vlc-server runtime: libvlc handles plus the embedded HTTP
// server. Constructed via New so libvlc init failures surface as errors
// instead of fatal-during-init side effects.
type Server struct {
	Version    string
	Player     *libvlc.Player
	Playlist   *libvlc.ListPlayer
	MediaList  *libvlc.MediaList
	VideoFiles []string

	http *http.Server
}

// New constructs a Server, initializing libvlc and loading media off disk.
// Returns a fully-initialized *Server on success, or (nil, err) on failure
// with any libvlc resources already allocated released before returning so
// the caller never has to clean up after a partial init.
func New(cfg Config) (*Server, error) {
	s := &Server{Version: cfg.Version}
	if err := s.initPlayer(); err != nil {
		s.releasePartial()
		return nil, err
	}
	return s, nil
}

// releasePartial releases any libvlc resources allocated before an init
// failure inside New. Mirrors Shutdown's release order (Player.Stop →
// Player.Release → libvlc.Release) and tolerates fields being nil because
// the failure may have happened before each was set.
func (s *Server) releasePartial() {
	if s.Player != nil {
		if err := s.Player.Stop(); err != nil {
			slog.Error("error stopping player during partial-init cleanup", "err", err)
		}
		if err := s.Player.Release(); err != nil {
			slog.Error("error releasing player during partial-init cleanup", "err", err)
		}
	}
	// libvlc.Release is safe to call even if startVLC failed before
	// libvlc.Init returned successfully — the underlying C ref-count
	// gate handles the no-op case. Always call it so that the libvlc
	// instance allocated in startVLC doesn't leak.
	if err := libvlc.Release(); err != nil {
		slog.Error("error releasing libvlc during partial-init cleanup", "err", err)
	}
}

// Start starts the web server. When ctx is canceled (e.g. SIGINT/SIGTERM
// via signal.NotifyContext) the server stops accepting new connections and
// waits up to shutdownTimeout for in-flight requests to complete before
// returning.
func (s *Server) Start(ctx context.Context) {
	slog.InfoContext(ctx, "starting VLC web server", "bind", c.Conf.VlcServerBindAddress)

	r := mux.NewRouter()

	// healthcheck endpoints
	//TODO: handle HEAD requests here too
	hp := r.PathPrefix("/health").Methods("GET", "HEAD").Subrouter()
	hp.Handle("/", tagged("/health/", s.healthHandler))
	hp.Handle("/live", tagged("/health/live", s.healthHandler))
	hp.Handle("/ready", tagged("/health/ready", s.healthHandler))

	// version endpoint — returns build metadata as JSON
	r.Handle("/version", tagged("/version", s.versionHandler)).Methods("GET", "HEAD")

	// vlc endpoints
	vlc := r.PathPrefix("/vlc").Methods("GET").Subrouter()
	vlc.Handle("/current", tagged("/vlc/current", s.vlcCurrentHandler))
	vlc.Handle("/play/{video}", tagged("/vlc/play/{video}", s.vlcPlayHandler))
	vlc.Handle("/random", tagged("/vlc/random", s.vlcRandomHandler))
	vlc.Handle("/back", tagged("/vlc/back", s.vlcBackHandler))
	vlc.Handle("/back/{n}", tagged("/vlc/back/{n}", s.vlcBackHandler))
	vlc.Handle("/skip", tagged("/vlc/skip", s.vlcSkipHandler))
	vlc.Handle("/skip/{n}", tagged("/vlc/skip/{n}", s.vlcSkipHandler))

	// onscreen endpoints now live in cmd/onscreens-server (separate binary,
	// separate port). vlc-server no longer serves /onscreens/* — clients
	// (chatbot, OBS browser sources) hit ONSCREENS_SERVER_HOST directly.

	// prometheus metrics endpoint
	r.Path("/metrics").Handler(tagged("/metrics", promhttp.Handler().ServeHTTP))

	// static assets
	r.Handle("/favicon.ico", tagged("/favicon.ico", s.faviconHandler)).Methods("GET")

	// catch everything else
	r.Handle("/", tagged("/", s.catchAllHandler))

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

	s.http = &http.Server{
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
		if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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
		if err := s.http.Shutdown(shutdownCtx); err != nil {
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
