package server

import (
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/davecgh/go-spew/spew"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	mytwitch "github.com/dmerrick/danalol-stream/pkg/twitch"
	"github.com/logrusorgru/aurora"
)

func handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		// healthcheck URL, for tools to verify the bot is alive
		if r.URL.Path == "/health" {
			fmt.Fprintf(w, "OK")

		} else if r.URL.Path == "/auth/twitch" {
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

	// someone tried a POST or a PUT or something
	default:
		fmt.Fprintf(w, "Only GET methods are supported.\n")
	}
}

// Start starts the web server
func Start() {
	log.Println("Starting web server")
	http.HandleFunc("/", auth(handle))
	//TODO: configurable port
	if err := http.ListenAndServe(":8080", nil); err != nil {
		terrors.Fatal(err, "couldn't start server")
	}
}

// https://stackoverflow.com/a/34405834
func auth(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, _ := r.BasicAuth()
		spew.Dump(user, pass)
		if !check(user, pass) {
			http.Error(w, "Unauthorized.", 401)
			return
		}
		fn(w, r)
	}
}

func check(u, p string) bool {
	if u == "trip" && p == "bot" {
		return true
	}
	return false
}
