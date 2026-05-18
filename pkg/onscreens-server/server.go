package onscreensServer

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/onscreens-server"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
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
	"encoding/json"
)

// versionTag is set by main via SetVersion; overridden at build time
// through `-ldflags "-X main.version=..."`.
var versionTag = "dev"

// SetVersion lets cmd/onscreens-server inject its build-time version
// string before the HTTP server starts.
func SetVersion(v string) {
	if v != "" {
		versionTag = v
	}
}

// Start brings up the HTTP listener serving all /onscreens/* routes plus
// the standard /health, /version, /metrics surface.
func Start() {
	slog.Info("starting onscreens-server web server", "bind", c.Conf.OnscreensServerBindAddress)

	r := mux.NewRouter()

	// healthcheck endpoints
	hp := r.PathPrefix("/health").Methods("GET", "HEAD").Subrouter()
	hp.Handle("/", tagged("/health/", healthHandler))
	hp.Handle("/live", tagged("/health/live", healthHandler))
	hp.Handle("/ready", tagged("/health/ready", healthHandler))

	// version endpoint — returns build metadata as JSON
	r.Handle("/version", tagged("/version", versionHandler)).Methods("GET", "HEAD")

	// onscreen endpoints
	osc := r.PathPrefix("/onscreens").Methods("GET").Subrouter()
	osc.Handle("/flag/{action}", tagged("/onscreens/flag/{action}", onscreensFlagHandler))
	osc.Handle("/gps/{action}", tagged("/onscreens/gps/{action}", onscreensGpsHandler))
	osc.Handle("/leaderboard/{action}", tagged("/onscreens/leaderboard/{action}", onscreensLeaderboardHandler))
	osc.Handle("/middle/{action}", tagged("/onscreens/middle/{action}", onscreensMiddleHandler))
	osc.Handle("/timewarp/{action}", tagged("/onscreens/timewarp/{action}", onscreensTimewarpHandler))
	// browser-source feeds: state JSON, per-onscreen HTML pages, and image assets.
	osc.Handle("/state.json", tagged("/onscreens/state.json", onscreensStateHandler))
	osc.Handle("/render/{name}", tagged("/onscreens/render/{name}", onscreensRenderHandler))
	osc.Handle("/asset/{name}", tagged("/onscreens/asset/{name}", onscreensAssetHandler))

	// prometheus metrics endpoint
	r.Path("/metrics").Handler(tagged("/metrics", promhttp.Handler().ServeHTTP))

	// catch everything else
	r.Handle("/", tagged("/", catchAllHandler))

	if c.Conf.Verbose {
		helpers.PrintAllRoutes(r)
	}

	app := negroni.Classic()

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

	srv := &http.Server{
		Addr:           c.Conf.OnscreensServerBindAddress,
		WriteTimeout:   time.Second * 15,
		ReadTimeout:    time.Second * 15,
		IdleTimeout:    time.Second * 60,
		MaxHeaderBytes: 1 << 20, // 1 MB
		Handler:        otelhttp.NewHandler(app, c.Conf.ServerType),
	}

	if err := srv.ListenAndServe(); err != nil {
		terrors.Fatal(err, "couldn't start server")
	}
}

// versionHandler returns build metadata as JSON. The tag comes from the
// build-time ldflag; sha + built_at are read from the binary's embedded
// VCS info (Go's automatic -buildvcs).
func versionHandler(w http.ResponseWriter, r *http.Request) {
	resp := struct {
		Tag     string `json:"tag"`
		Sha     string `json:"sha"`
		BuiltAt string `json:"built_at"`
	}{Tag: versionTag}

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				resp.Sha = s.Value
			case "vcs.time":
				resp.BuiltAt = s.Value
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		terrors.Log(err, "couldn't encode version response")
	}
}

// tagged wraps a HandlerFunc so the http.route attribute is set on metrics
// (via otelhttp.Labeler) and traces (via the active span). Negroni doesn't
// surface the underlying mux route template to the otelhttp middleware, so
// each registration declares it.
func tagged(route string, h http.HandlerFunc) http.Handler {
	attr := semconv.HTTPRoute(route)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		if labeler, ok := otelhttp.LabelerFromContext(ctx); ok {
			labeler.Add(attr)
		}
		trace.SpanFromContext(ctx).SetAttributes(attr)
		h(w, req)
	})
}
