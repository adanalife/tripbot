package server

import (
	"errors"
	"fmt"
	"log"
	"net/http"

	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	mytwitch "github.com/dmerrick/danalol-stream/pkg/twitch"
	"github.com/logrusorgru/aurora"
)

func handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/auth/callback" {
		http.Error(w, "404 not found", http.StatusNotFound)
		log.Println("someone tried hitting", r.URL.Path)
		return
	}

	switch r.Method {
	case "GET":
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

	case "POST":
		fmt.Fprintf(w, "Perhaps you meant to make a GET request?\n")
	default:
		fmt.Fprintf(w, "Only GET and POST methods are supported.\n")
	}
}

func Start() {
	log.Println("Starting server...")
	http.HandleFunc("/", handle)
	//TODO: configurable port
	if err := http.ListenAndServe(":8080", nil); err != nil {
		terrors.Log(err, "couldn't start server")
	}
}
