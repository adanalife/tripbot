package server

import (
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
			msg := "Param 'code' is missing"
			log.Println(msg)
			//TODO: better error than StatusNotFound
			http.Error(w, msg, http.StatusNotFound)
			return
		}
		code := string(codes[0])
		mytwitch.GenerateUserAccessToken(code)
		//TODO: consider removing this line
		log.Println("Code received from Twitch:", aurora.Cyan(code))
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
