package server

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/adanalife/tripbot/pkg/chatbot"
	"github.com/adanalife/tripbot/pkg/config"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
	"github.com/adanalife/tripbot/pkg/users"
	"github.com/gorilla/mux"
	"github.com/logrusorgru/aurora"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/urfave/negroni"
	"golang.org/x/crypto/acme/autocert"
)

var certManager autocert.Manager
var server *http.Server

//TODO: write real healthchecks for ready vs live
// healthcheck URL, for tools to verify the bot is alive
func healthHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "OK")
}

// twitch issues a request here when creating a new webhook subscription
func webhooksTwitchHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("got webhook challenge request at", r.URL.Path)
	// exit early if we've disabled webhooks
	if config.DisableTwitchWebhooks {
		http.Error(w, "501 not implemented", http.StatusNotImplemented)
		return
	}

	challenge, ok := r.URL.Query()["hub.challenge"]
	if !ok || len(challenge[0]) < 1 {
		terrors.Log(nil, "something went wrong with the challenge")
		log.Printf("%#v", r.URL.Query())
		http.Error(w, "404 not found", http.StatusNotFound)
		return
	}
	log.Println("returning challenge")
	fmt.Fprintf(w, string(challenge[0]))
}

// this endpoint returns private twitch access tokens
func authTwitchHandler(w http.ResponseWriter, r *http.Request) {
	secret, ok := r.URL.Query()["auth"]
	if !ok || !isValidSecret(secret[0]) {
		http.Error(w, "404 not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, twitchAuthJSON())
}

// oauth callback URL, requests come from Twitch and have a special code
// we then use that code to generate a User Access Token
func authCallbackHandler(w http.ResponseWriter, r *http.Request) {
	codes, ok := r.URL.Query()["code"]

	if !ok || len(codes[0]) < 1 {
		msg := "no code in response from twitch"
		terrors.Log(errors.New("code missing"), msg)
		//TODO: better error than StatusNotFound (404)
		http.Error(w, msg, http.StatusNotFound)
		return
	}
	code := string(codes[0])

	log.Println(aurora.Cyan("successfully received token from twitch!"))
	// use the code to generate an access token
	mytwitch.GenerateUserAccessToken(code)

	//TODO: return a pretty HTML page here (black background, logo, etc)
	fmt.Fprintf(w, "Success!")
}

// return a favicon if anyone asks for one
func faviconHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "assets/favicon.ico")
}

// user webhooks are received via POST at this url
//TODO: we can use helix.GetWebhookTopicFromRequest() and share a webhooks URL
func webhooksTwitchUsersFollowsHandler(w http.ResponseWriter, r *http.Request) {
	if config.DisableTwitchWebhooks {
		http.Error(w, "501 not implemented", http.StatusNotImplemented)
		return
	}

	resp, err := decodeFollowWebhookResponse(r)
	if err != nil {
		terrors.Log(err, "error decoding follow webhook")
		//TODO: better error
		http.Error(w, "404 not found", http.StatusNotFound)
		return
	}

	for _, follower := range resp.Data.Follows {
		username := follower.FromName
		log.Println("got webhook for new follower:", username)
		users.LoginIfNecessary(username)
		// announce new follower in chat
		chatbot.AnnounceNewFollower(username)
	}

	fmt.Fprintf(w, "OK")
}

// these are sent when users subscribe
func webhooksTwitchSubscriptionsEventsHandler(w http.ResponseWriter, r *http.Request) {
	if config.DisableTwitchWebhooks {
		http.Error(w, "501 not implemented", http.StatusNotImplemented)
		return
	}

	resp, err := decodeSubscriptionWebhookResponse(r)
	if err != nil {
		terrors.Log(err, "error decoding subscription webhook")
		//TODO: better error
		http.Error(w, "404 not found", http.StatusNotFound)
		return
	}

	for _, event := range resp.Data.Events {
		username := event.Subscription.UserName
		log.Println("got webhook for new sub:", username)
		users.LoginIfNecessary(username)
		// announce new sub in chat
		chatbot.AnnounceSubscriber(event.Subscription)
	}

	// update the internal subscribers list
	mytwitch.GetSubscribers()

	fmt.Fprintf(w, "OK")
}

//TODO: consider adding routes to control MPD
func catchAllHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		http.Error(w, "404 not found", http.StatusNotFound)
		log.Println("someone tried hitting", r.URL.Path)
		return

	case "POST":
		// someone tried to make a post and we dont know what to do with it
		http.Error(w, "404 not found", http.StatusNotFound)
		log.Println("someone tried posting to", r.URL.Path)
		return
	// someone tried a PUT or a DELETE or something
	default:
		fmt.Fprintf(w, "Only GET/POST methods are supported.\n")
	}
}

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

// isValidSecret returns true if the given secret matches the configured oen
func isValidSecret(secret string) bool {
	return len(secret) < 1 || secret != config.TripbotHttpAuth
}
