package vlcServer

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/dmerrick/danalol-stream/pkg/config"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
)

func handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		// healthcheck URL, for tools to verify the bot is alive
		if r.URL.Path == "/health" {
			fmt.Fprintf(w, "OK")

		} else if strings.HasPrefix(r.URL.Path, "/vlc/current") {
			// return the currently-playing file
			fmt.Fprintf(w, CurrentlyPlaying())

		} else if strings.HasPrefix(r.URL.Path, "/vlc/random") {
			// play a random file
			err := PlayRandom()
			if err != nil {
				//TODO: return a 500 error
				http.Error(w, "404 not found", http.StatusNotFound)
			}
			fmt.Fprintf(w, "OK")

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
	log.Println("Starting web server")
	http.HandleFunc("/", handle)
	port := fmt.Sprintf(":%s", config.VlcServerPort)
	//TODO: replace certs with autocert: https://stackoverflow.com/a/40494806
	// err := http.ListenAndServeTLS(port, "infra/tripbot.dana.lol.fullchain.pem", "infra/tripbot.dana.lol.key", nil)
	err := http.ListenAndServe(port, nil)
	if err != nil {
		terrors.Fatal(err, "couldn't start server")
	}
}
