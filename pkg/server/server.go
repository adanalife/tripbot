package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/davecgh/go-spew/spew"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	mytwitch "github.com/dmerrick/danalol-stream/pkg/twitch"
	"github.com/logrusorgru/aurora"
	"github.com/nicklaw5/helix"
)

func handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		// healthcheck URL, for tools to verify the bot is alive
		if r.URL.Path == "/health" {
			fmt.Fprintf(w, "OK")

		} else if r.URL.Path == "/webhooks" {
			log.Println("got request to /webhooks")
			challenge, ok := r.URL.Query()["hub.challenge"]
			if !ok || len(challenge[0]) < 1 {
				http.Error(w, "404 not found", http.StatusNotFound)
				return
			}
			log.Println("returning challenge")
			fmt.Fprintf(w, string(challenge[0]))

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
		// healthcheck URL, for tools to verify the bot is alive
		if r.URL.Path == "/webhooks" {
			// resp := &helix.Response{}
			resp := &helix.UsersFollowsResponse{}
			bodyBytes, err := ioutil.ReadAll(r.Body)
			if err != nil {
				log.Println("something went wrong")
				//TODO: better error
				http.Error(w, "404 not found", http.StatusNotFound)
				return
			}

			fmt.Println(string(bodyBytes) + "\n")

			// Only attempt to decode the response if we have a response we can handle
			if len(bodyBytes) > 0 && resp.StatusCode < http.StatusInternalServerError {
				if resp.StatusCode < http.StatusBadRequest {
					// if resp.Data != nil && resp.StatusCode < http.StatusBadRequest {
					// Successful request
					err = json.Unmarshal(bodyBytes, &resp.Data)
				} else {
					// Failed request
					err = json.Unmarshal(bodyBytes, &resp)
				}

				if err != nil {
					log.Println(fmt.Errorf("Failed to decode API response: %s", err.Error()))
					return
				}
			}

			spew.Dump(resp.Data.Follows)

			fmt.Fprintf(w, "OK")
		} else {
			http.Error(w, "404 not found", http.StatusNotFound)
			log.Println("someone tried hitting", r.URL.Path)
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
	if err := http.ListenAndServe(":8080", nil); err != nil {
		terrors.Fatal(err, "couldn't start server")
	}
}
