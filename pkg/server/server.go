package server

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/dmerrick/danalol-stream/pkg/chatbot"
	"github.com/dmerrick/danalol-stream/pkg/config"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	mytwitch "github.com/dmerrick/danalol-stream/pkg/twitch"
	"github.com/dmerrick/danalol-stream/pkg/users"
	"github.com/logrusorgru/aurora"
	"golang.org/x/crypto/acme/autocert"
)

var certManager autocert.Manager
var server *http.Server

func init() {
	port := fmt.Sprintf(":%s", config.TripbotServerPort)

	// automatically create SSL certs
	// using let's encrypt
	// c.p. https://stackoverflow.com/a/40494806
	certManager = autocert.Manager{
		Prompt: autocert.AcceptTOS,
		//TODO: make this a Config var
		HostPolicy: autocert.HostWhitelist("tripbot.dana.lol"),
		Cache:      autocert.DirCache("infra/certs"),
	}
	server = &http.Server{
		Addr: port,
		TLSConfig: &tls.Config{
			GetCertificate: certManager.GetCertificate,
		},
	}
}

//TODO: consider adding routes to control MPD
func handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		// healthcheck URL, for tools to verify the bot is alive
		if r.URL.Path == "/health" {
			fmt.Fprintf(w, "OK")

			// twitch issues a request here when creating a new webhook subscription
		} else if strings.HasPrefix(r.URL.Path, "/webhooks/twitch") {
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

			// this endpoint returns private twitch access tokens
		} else if r.URL.Path == "/auth/twitch" {
			secret, ok := r.URL.Query()["auth"]
			if !ok || !isValidSecret(secret[0]) {
				http.Error(w, "404 not found", http.StatusNotFound)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, twitchAuthJSON())

			// oauth callback URL, requests come from Twitch and have a special code
			// we then use that code to generate a User Access Token
		} else if r.URL.Path == "/auth/callback" {
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

			fmt.Fprintf(w, "Success!")
			return

			// some other URL was used
		} else {
			http.Error(w, "404 not found", http.StatusNotFound)
			log.Println("someone tried hitting", r.URL.Path)
			return
		}

	case "POST":
		// user webhooks are received via POST at this url
		//TODO: we can use helix.GetWebhookTopicFromRequest() and
		// share a webhooks URL
		if r.URL.Path == "/webhooks/twitch/users/follows" {

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

			// these are sent when users subscribe
		} else if r.URL.Path == "/webhooks/twitch/subscriptions/events" {

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
		} else {
			// someone tried to make a post and we dont know what to do with it
			http.Error(w, "404 not found", http.StatusNotFound)
			log.Println("someone tried posting to", r.URL.Path)
			return
		}
	// someone tried a PUT or a DELETE or something
	default:
		fmt.Fprintf(w, "Only GET/POST methods are supported.\n")
	}
}

// Start starts the web server
func Start() {
	log.Println("Starting web server")
	http.HandleFunc("/", handle)

	// start http server (for http->https redirect)
	//TODO: config port
	go http.ListenAndServe(":8888", certManager.HTTPHandler(nil))

	err := server.ListenAndServeTLS("", "")
	if err != nil {
		terrors.Fatal(err, "couldn't start server")
	}
}

// isValidSecret returns true if the given secret matches the configured oen
func isValidSecret(secret string) bool {
	return len(secret) < 1 || secret != config.TripbotHttpAuth
}
