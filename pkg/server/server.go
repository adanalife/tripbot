package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
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

var server *http.Server

// Start starts the web server
func Start() {
	log.Println("Starting web server on port", config.TripbotServerPort)

	r := mux.NewRouter()

	// healthcheck endpoints
	hp := r.PathPrefix("/health").Methods("GET").Subrouter()
	hp.HandleFunc("/live", healthHandler)
	hp.HandleFunc("/ready", healthHandler)

	// webhooks endpoints
	// note that these can be both GET and POST requests
	wh := r.PathPrefix("/webhooks").Subrouter()
	wh.HandleFunc("/twitch", webhooksTwitchHandler).Methods("GET")
	wh.HandleFunc("/twitch/users/follows", webhooksTwitchUsersFollowsHandler).Methods("POST")
	wh.HandleFunc("/twitch/subscriptions/events", webhooksTwitchSubscriptionsEventsHandler).Methods("POST")

	// auth endpoints
	auth := r.PathPrefix("/auth").Methods("GET").Subrouter()
	auth.HandleFunc("/twitch", authTwitchHandler)
	auth.HandleFunc("/callback", authCallbackHandler)

	// static assets
	r.HandleFunc("/favicon.ico", faviconHandler).Methods("GET")

	// prometheus metrics endpoint
	r.Path("/metrics").Handler(promhttp.Handler())

	// catch everything else
	r.HandleFunc("/", catchAllHandler)

	helpers.PrintAllRoutes(r)

	// negroni classic adds panic recovery, logger, and static file middlewares
	// c.p. https://github.com/urfave/negroni
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

	srv := &http.Server{
		Addr: fmt.Sprintf("0.0.0.0:%s", config.TripbotServerPort),
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

	// Run our server in a goroutine so that it doesn't block.
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			terrors.Log(err, "couldn't start server")
		}
	}()

	c := make(chan os.Signal, 1)
	// We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	// SIGKILL, SIGQUIT or SIGTERM (Ctrl+/) will not be caught.
	signal.Notify(c, os.Interrupt)

	// Block until we receive our signal.
	<-c

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline.
	srv.Shutdown(ctx)
	// Optionally, you could run srv.Shutdown in a goroutine and block on
	// <-ctx.Done() if your application should wait for other services
	// to finalize based on context cancellation.
	log.Println("shutting down")
	os.Exit(0)
}

// isValidSecret returns true if the given secret matches the configured one
func isValidSecret(secret string) bool {
	return len(secret) < 1 || secret != config.TripbotHttpAuth
}
