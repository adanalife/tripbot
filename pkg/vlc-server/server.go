package vlcServer

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/adanalife/tripbot/pkg/config"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
	sentrynegroni "github.com/getsentry/sentry-go/negroni"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	metrics "github.com/slok/go-http-metrics/metrics/prometheus"
	"github.com/slok/go-http-metrics/middleware"
	negronimiddleware "github.com/slok/go-http-metrics/middleware/negroni"
	"github.com/unrolled/secure"
	"github.com/urfave/negroni"
)

// Start starts the web server
func Start() {
	log.Println("Starting VLC web server on host", config.VlcServerHost)

	r := mux.NewRouter()

	// healthcheck endpoints
	//TODO: handle HEAD requests here too
	hp := r.PathPrefix("/health").Methods("GET").Subrouter()
	hp.HandleFunc("/", healthHandler)
	hp.HandleFunc("/live", healthHandler)
	hp.HandleFunc("/ready", healthHandler)

	// vlc endpoints
	vlc := r.PathPrefix("/vlc").Methods("GET").Subrouter()
	vlc.HandleFunc("/current", vlcCurrentHandler)
	vlc.HandleFunc("/play/{video}", vlcPlayHandler)
	vlc.HandleFunc("/random", vlcRandomHandler)
	vlc.HandleFunc("/back", vlcBackHandler)
	vlc.HandleFunc("/back/{n}", vlcBackHandler)
	vlc.HandleFunc("/skip", vlcSkipHandler)
	vlc.HandleFunc("/skip/{n}", vlcSkipHandler)

	// onscreen endpoints
	osc := r.PathPrefix("/onscreens").Methods("GET").Subrouter()
	//TODO: add state variable
	osc.HandleFunc("/flag/{action}", onscreensFlagHandler)
	osc.HandleFunc("/gps/{action}", onscreensGpsHandler)
	osc.HandleFunc("/gps/{action}", onscreensGpsHandler)
	osc.HandleFunc("/leaderboard/{action}", onscreensLeaderboardHandler)
	osc.HandleFunc("/middle/{action}", onscreensMiddleHandler)
	osc.HandleFunc("/middle/{action}", onscreensMiddleHandler)
	osc.HandleFunc("/timewarp/{action}", onscreensTimewarpHandler)

	// prometheus metrics endpoint
	r.Path("/metrics").Handler(promhttp.Handler())

	// static assets
	r.HandleFunc("/favicon.ico", faviconHandler).Methods("GET")

	// catch everything else
	r.HandleFunc("/", catchAllHandler)

	if config.Verbose {
		helpers.PrintAllRoutes(r)
	}

	// negroni classic adds panic recovery, logger, and static file middlewares
	// c.p. https://github.com/urfave/negroni
	//TODO: consider adding HTMLPanicFormatter
	app := negroni.Classic()

	// attach http-metrics (prometheus) middleware
	metricsMw := middleware.New(middleware.Config{
		Recorder: metrics.NewRecorder(metrics.Config{}),
		Service:  config.ServerType,
	})
	app.Use(negronimiddleware.Handler("", metricsMw))

	// attach security middleware
	secureMw := secure.New(secure.Options{
		FrameDeny:     true,
		IsDevelopment: config.IsDevelopment(),
	})
	app.Use(negroni.HandlerFunc(secureMw.HandlerFuncWithNext))

	// attach Sentry middleware (for reporting exceptions)
	app.Use(sentrynegroni.New(sentrynegroni.Options{}))

	// attaching routes to handler happens last
	app.UseHandler(r)

	//TODO: error if there's no colon to split on
	port := strings.Split(config.VlcServerHost, ":")[1]

	srv := &http.Server{
		Addr: fmt.Sprintf("0.0.0.0:%s", port),
		// Good practice to set timeouts to avoid Slowloris attacks.
		WriteTimeout:   time.Second * 15,
		ReadTimeout:    time.Second * 15,
		IdleTimeout:    time.Second * 60,
		MaxHeaderBytes: 1 << 20, // 1 MB
		Handler:        app,     // Pass our instance of negroni in
	}

	//TODO: add graceful shutdown
	if err := srv.ListenAndServe(); err != nil {
		terrors.Fatal(err, "couldn't start server")
	}
}
