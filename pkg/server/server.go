package server

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/dmerrick/danalol-stream/pkg/tripbot"
	mytwitch "github.com/dmerrick/danalol-stream/pkg/twitch"
	"github.com/logrusorgru/aurora"
)

func handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		// healthcheck URL, for tools to verify the bot is alive
		if r.URL.Path == "/health" {
			fmt.Fprintf(w, "OK")

			// twitch issues a request here when creating a new webhook subscription
		} else if strings.HasPrefix(r.URL.Path, "/webhooks/twitch") {
			log.Println("got webhook challenge request at", r.URL.Path)
			challenge, ok := r.URL.Query()["hub.challenge"]
			if !ok || len(challenge[0]) < 1 {
				http.Error(w, "404 not found", http.StatusNotFound)
				return
			}
			log.Println("returning challenge")
			fmt.Fprintf(w, string(challenge[0]))

			// this endpoint returns private twitch access tokens
		} else if r.URL.Path == "/auth/twitch" {
			secret, ok := r.URL.Query()["auth"]
			//TODO: more secure password (lol)
			if !ok || len(secret[0]) < 1 || secret[0] != "yes" {
				http.Error(w, "404 not found", http.StatusNotFound)
				return
			}

			authJSON := TwitchAuthJSON()
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, authJSON)

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
				// announce new follower in chat
				tripbot.AnnounceNewFollower(username)
			}

			fmt.Fprintf(w, "OK")

			// these are sent when users subscribe
		} else if r.URL.Path == "/webhooks/twitch/subscriptions/events" {

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
				// announce new sub in chat
				tripbot.AnnounceSubscriber(event.Subscription)
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
	//TODO: configurable port
	//TODO: replace certs with autocert: https://stackoverflow.com/a/40494806
	err := http.ListenAndServeTLS(":8080", "configs/tripbot.dana.lol.fullchain.pem", "configs/tripbot.dana.lol.key", nil)
	// err := http.ListenAndServe(":8080", nil)
	if err != nil {
		terrors.Fatal(err, "couldn't start server")
	}
}
