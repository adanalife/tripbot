package vlcServer

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/dmerrick/tripbot/pkg/config"
	terrors "github.com/dmerrick/tripbot/pkg/errors"
)

func handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		// healthcheck URL, for tools to verify the bot is alive
		if r.URL.Path == "/health" {
			fmt.Fprintf(w, "OK")

		} else if strings.HasPrefix(r.URL.Path, "/vlc/current") {
			// return the currently-playing file
			fmt.Fprintf(w, currentlyPlaying())

		} else if strings.HasPrefix(r.URL.Path, "/vlc/play") {
			videoFile, ok := r.URL.Query()["video"]
			if !ok || len(videoFile) > 1 {
				//TODO: eventually this could just play instead of hard-requiring a param
				http.Error(w, "422 unprocessable entity", http.StatusUnprocessableEntity)
				return
			}

			spew.Dump(videoFile)
			playVideoFile(videoFile[0])

			//TODO: better response
			fmt.Fprintf(w, "OK")

		} else if strings.HasPrefix(r.URL.Path, "/vlc/back") {
			num, ok := r.URL.Query()["n"]
			if !ok || len(num) > 1 {
				back(1)
				return
			}
			i, err := strconv.Atoi(num[0])
			if err != nil {
				terrors.Log(err, "couldn't convert input to int")
				http.Error(w, "422 unprocessable entity", http.StatusUnprocessableEntity)
				return
			}

			back(i)

			//TODO: better response
			fmt.Fprintf(w, "OK")

		} else if strings.HasPrefix(r.URL.Path, "/vlc/skip") {
			num, ok := r.URL.Query()["n"]
			if !ok || len(num) > 1 {
				skip(1)
				return
			}
			i, err := strconv.Atoi(num[0])
			if err != nil {
				terrors.Log(err, "couldn't convert input to int")
				http.Error(w, "422 unprocessable entity", http.StatusUnprocessableEntity)
				return
			}

			skip(i)

			//TODO: better response
			fmt.Fprintf(w, "OK")

		} else if strings.HasPrefix(r.URL.Path, "/vlc/random") {
			// play a random file
			err := PlayRandom()
			if err != nil {
				//TODO: return a 500 error
				http.Error(w, "404 not found", http.StatusNotFound)
			}
			fmt.Fprintf(w, "OK")

			// return a favicon if anyone asks for one
		} else if r.URL.Path == "/favicon.ico" {
			http.ServeFile(w, r, "assets/favicon.ico")

			// some other URL was used
		} else {
			http.Error(w, "404 not found", http.StatusNotFound)
			log.Println("someone tried hitting", r.URL.Path)
			return
		}

	// someone tried a PUT or a DELETE or something
	default:
		fmt.Fprintf(w, "Only GET methods are supported.\n")
	}
}

// Start starts the web server
func Start() {
	log.Println("Starting VLC web server on host", config.VlcServerHost)
	http.HandleFunc("/", handle)

	// ListenAndServe() wants a port in the format ":NUM"
	//TODO: error if there's no colon to split on
	port := ":" + strings.Split(config.VlcServerHost, ":")[1]
	//TODO: replace certs with autocert: https://stackoverflow.com/a/40494806
	// err := http.ListenAndServeTLS(port, "infra/tripbot.dana.lol.fullchain.pem", "infra/tripbot.dana.lol.key", nil)
	err := http.ListenAndServe(port, nil)
	if err != nil {
		terrors.Fatal(err, "couldn't start server")
	}
}
