package onscreensServer

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/onscreens-server"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/httpmw"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Config bundles what cmd/onscreens-server passes into New.
type Config struct {
	// Version is the build-time tag returned by /version. Typically set
	// from cmd/onscreens-server's `main.version` var, which is overridden
	// via `-ldflags "-X main.version=..."`.
	Version string
	// Conf is the process config, loaded once in main via config.Load().
	Conf *c.OnscreensServerConfig
}

// Server owns the onscreen singletons and the HTTP listener that serves
// them. Construct via New; call Start to block on the HTTP listener.
type Server struct {
	Version string
	cfg     *c.OnscreensServerConfig

	GPS          *Onscreen
	Leaderboard  *Onscreen
	LeftRotator  *Onscreen
	MiddleText   *Onscreen
	RightRotator *Onscreen
	Timewarp     *Onscreen

	// The corner rotators behind LeftRotator / RightRotator. Kept so copy
	// edited in the admin console, arriving over NATS, can be swapped into the
	// live pools.
	left  *rotator
	right *rotator

	// timewarpNoBackground is the last warp's "strip the opaque cover" choice,
	// carried on timewarp.show and served in state.json for the browser source
	// to apply. Server-level rather than a field on the Onscreen: it's styling
	// for one overlay, not state every overlay has.
	timewarpNoBackground atomic.Bool

	// middleExpiry is the pending auto-hide for the middle text, when the show
	// that set it asked for one. Held so the next show can cancel it —
	// otherwise the previous message's timer hides the new one early.
	middleExpiryMu sync.Mutex
	middleExpiry   *time.Timer

	http *http.Server
}

// New constructs a *Server with all six onscreens initialized and
// their background loops (rotators, expiry sweepers) running. It does
// not bind any sockets — Start does that.
func New(cfg Config) *Server {
	version := cfg.Version
	if version == "" {
		version = "dev"
	}
	left, right := startRotators(cfg.Conf)
	return &Server{
		Version:      version,
		cfg:          cfg.Conf,
		GPS:          newGPSOnscreen(),
		Leaderboard:  newLeaderboardOnscreen(),
		LeftRotator:  left.osc,
		MiddleText:   newMiddleText(),
		RightRotator: right.osc,
		Timewarp:     newTimewarp(),
		left:         left,
		right:        right,
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
	slog.InfoContext(ctx, "starting onscreens-server web server", "bind", s.cfg.OnscreensServerBindAddress)

	// Attach NATS subscribers. No-op when the natsclient singleton is
	// nil (NATS_URL unset); HTTP remains the sole transport in that case.
	s.StartNATSSubscribers(ctx)

	r := mux.NewRouter()

	// healthcheck endpoints
	hp := r.PathPrefix("/health").Methods("GET", "HEAD").Subrouter()
	hp.Handle("/", httpmw.Tagged("/health/", healthHandler))
	hp.Handle("/live", httpmw.Tagged("/health/live", healthHandler))
	hp.Handle("/ready", httpmw.Tagged("/health/ready", healthHandler))

	// version endpoint — returns build metadata as JSON
	r.Handle("/version", httpmw.Tagged("/version", s.versionHandler)).Methods("GET", "HEAD")

	// onscreen endpoints — commands (middle / leaderboard / timewarp / gps)
	// arrive over NATS now (see nats.go); only the browser-source feeds
	// remain on HTTP: state JSON, per-onscreen HTML pages, and image assets.
	osc := r.PathPrefix("/onscreens").Methods("GET").Subrouter()
	osc.Handle("/state.json", httpmw.Tagged("/onscreens/state.json", s.onscreensStateHandler))
	osc.Handle("/render/{name}", httpmw.Tagged("/onscreens/render/{name}", s.onscreensRenderHandler))
	osc.Handle("/asset/{name}", httpmw.Tagged("/onscreens/asset/{name}", s.onscreensAssetHandler))

	// admin actions — no app-layer auth gate. onscreens-server publishes no
	// Ingress, so this is reachable only in-cluster (Service DNS) or through a
	// port-forward. /admin/shutdown is the admin panel's "restart
	// onscreens-server" surface; the shared handler SIGTERMs the process and
	// k8s restartPolicy: Always brings the pod back.
	admin := r.PathPrefix("/admin").Methods("POST").Subrouter()
	admin.Handle("/shutdown", httpmw.Tagged("/admin/shutdown", httpmw.ShutdownHandler()))

	// prometheus metrics endpoint
	r.Path("/metrics").Handler(httpmw.Tagged("/metrics", promhttp.Handler().ServeHTTP))

	// catch everything else
	r.Handle("/", httpmw.Tagged("/", catchAllHandler))

	if s.cfg.Verbose {
		helpers.PrintAllRoutes(r)
	}

	app := httpmw.NewChain(s.cfg.ServerType, s.cfg.IsDevelopment(), func(any) { instrumentation.HTTPPanics.Inc(s.cfg.ServerType) })

	app.UseHandler(r)

	s.http = &http.Server{
		Addr:           s.cfg.OnscreensServerBindAddress,
		WriteTimeout:   time.Second * 15,
		ReadTimeout:    time.Second * 15,
		IdleTimeout:    time.Second * 60,
		MaxHeaderBytes: 1 << 20, // 1 MB
		Handler:        otelhttp.NewHandler(app, s.cfg.ServerType),
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

// RestoreFromJetStream restores the state a restart would otherwise lose —
// the permanent middle-text overlay and the console-edited rotator copy — from
// their JetStream last-value caches. Run it from the NATS on-connect callback
// (see cmd/onscreens-server), not from Start: the client connects
// asynchronously and keeps retrying, so a restore issued before the connection
// is up times out against a server that isn't there yet, while the value
// still sits in the stream. Best-effort: a nil JetStream (NATS off) or an
// empty cache leaves each overlay at its constructed default.
func (s *Server) RestoreFromJetStream(ctx context.Context) {
	s.RestoreMiddleText(ctx)
	s.RestoreRotatorCopy(ctx)
}

// Shutdown ends every overlay's background loops (the expiry sweepers and the
// two rotators), then gracefully stops the HTTP listener, allowing in-flight
// requests up to ctx's deadline to complete before closing connections.
// Returns the error from http.Server.Shutdown so callers can log or act
// on a timeout. Safe to call more than once, once Start has returned (or
// concurrently while Start is blocked on ctx.Done()); on a Server whose Start
// never ran, only the loops are stopped.
func (s *Server) Shutdown(ctx context.Context) error {
	for _, osc := range s.all() {
		osc.stop()
	}
	if s.http == nil {
		return nil
	}
	return s.http.Shutdown(ctx)
}

// versionHandler returns build metadata as JSON; see httpmw.WriteVersion.
// The tag comes from Config{Version}, threaded into the Server.
func (s *Server) versionHandler(w http.ResponseWriter, r *http.Request) {
	httpmw.WriteVersion(w, r, s.Version)
}
