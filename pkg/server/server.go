package server

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/adanalife/tripbot/pkg/config"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
	sentrynegroni "github.com/getsentry/sentry-go/negroni"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/urfave/negroni"
	"golang.org/x/crypto/acme/autocert"
)

var certManager autocert.Manager
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
	//TODO: consider adding HTMLPanicFormatter
	app := negroni.Classic()

	// attach prometheus middleware
	app.Use(negroni.HandlerFunc(helpers.PrometheusMiddleware))
	// attach sentry middleware
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
}

// isValidSecret returns true if the given secret matches the configured one
func isValidSecret(secret string) bool {
	return len(secret) < 1 || secret != config.TripbotHttpAuth
}
