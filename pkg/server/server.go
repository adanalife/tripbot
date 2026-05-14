package server

import (
	"fmt"
	"log"
	"net/http"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
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
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

var server *http.Server

// Start starts the web server
func Start() {
	log.Println("Starting web server on port", c.Conf.TripbotServerPort)

	r := mux.NewRouter()

	// healthcheck endpoints
	hp := r.PathPrefix("/health").Methods("GET", "HEAD").Subrouter()
	hp.Handle("/live", tagged("/health/live", healthHandler))
	hp.Handle("/ready", tagged("/health/ready", healthHandler))

	// version endpoint — returns build metadata as JSON
	r.Handle("/version", tagged("/version", versionHandler)).Methods("GET", "HEAD")

	// webhooks endpoints
	// note that these can be both GET and POST requests
	wh := r.PathPrefix("/webhooks").Subrouter()
	wh.Handle("/twitch", tagged("/webhooks/twitch", webhooksTwitchHandler)).Methods("GET")
	wh.Handle("/twitch/users/follows", tagged("/webhooks/twitch/users/follows", webhooksTwitchUsersFollowsHandler)).Methods("POST")
	wh.Handle("/twitch/subscriptions/events", tagged("/webhooks/twitch/subscriptions/events", webhooksTwitchSubscriptionsEventsHandler)).Methods("POST")

	// auth endpoints
	auth := r.PathPrefix("/auth").Methods("GET").Subrouter()
	auth.Handle("/init", tagged("/auth/init", authInitHandler))
	auth.Handle("/callback", tagged("/auth/callback", authCallbackHandler))

	// static assets
	r.Handle("/favicon.ico", tagged("/favicon.ico", faviconHandler)).Methods("GET")

	// prometheus metrics endpoint
	r.Path("/metrics").Handler(tagged("/metrics", promhttp.Handler().ServeHTTP))

	// catch everything else
	r.Handle("/", tagged("/", catchAllHandler))

	if c.Conf.Verbose {
		helpers.PrintAllRoutes(r)
	}

	// negroni classic adds panic recovery, logger, and static file middlewares
	// c.p. https://github.com/urfave/negroni
	app := negroni.Classic()

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
		// Good practice to set timeouts to avoid Slowloris attacks.
		WriteTimeout:   time.Second * 15,
		ReadTimeout:    time.Second * 15,
		IdleTimeout:    time.Second * 60,
		MaxHeaderBytes: 1 << 20, // 1 MB
		Handler:        otelhttp.NewHandler(app, c.Conf.ServerType),
	}

	//TODO: add graceful shutdown
	if err := srv.ListenAndServe(); err != nil {
		terrors.Fatal(err, "couldn't start server")
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
