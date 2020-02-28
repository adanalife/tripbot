package vlcServer

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
)

//TODO: consider adding routes to control MPD
func handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		// healthcheck URL, for tools to verify the bot is alive
		if r.URL.Path == "/health" {
			fmt.Fprintf(w, "OK")

		} else if strings.HasPrefix(r.URL.Path, "/vlc/play") {
			//TODO: do something here

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
	//TODO: configurable port
	//TODO: replace certs with autocert: https://stackoverflow.com/a/40494806
	// err := http.ListenAndServeTLS(":8080", "infra/tripbot.dana.lol.fullchain.pem", "infra/tripbot.dana.lol.key", nil)
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		terrors.Fatal(err, "couldn't start server")
	}
}
