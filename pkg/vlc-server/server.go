package vlcServer

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/adanalife/tripbot/pkg/config"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
	onscreensServer "github.com/adanalife/tripbot/pkg/onscreens-server"
	"github.com/davecgh/go-spew/spew"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// healthcheck URL, for tools to verify the stream is alive
func healthHandler(w http.ResponseWriter, r *http.Request) {
	//TODO: rewrite this as a handler
	healthCheck(w)
}

func vlcCurrentHandler(w http.ResponseWriter, r *http.Request) {
	// return the currently-playing file
	fmt.Fprintf(w, currentlyPlaying())
}

func vlcPlayHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	spew.Dump(vars)
	spew.Dump(vars["video"])

	videoFile := vars["video"]
	//videoFile, ok := r.URL.Query()["video"]
	//if !ok || len(videoFile) > 1 {
	//	//TODO: eventually this could just play instead of hard-requiring a param
	//	http.Error(w, "417 expectation failed", http.StatusExpectationFailed)
	//	return
	//}

	spew.Dump(videoFile)
	playVideoFile(videoFile[0])

	//TODO: better response
	fmt.Fprintf(w, "OK")
}

func vlcBackHandler(w http.ResponseWriter, r *http.Request) {
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

}

func vlcSkipHandler(w http.ResponseWriter, r *http.Request) {
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
}

func vlcRandomHandler(w http.ResponseWriter, r *http.Request) {
	// play a random file
	err := PlayRandom()
	if err != nil {
		http.Error(w, "error playing random", http.StatusInternalServerError)
	}
	fmt.Fprintf(w, "OK")
}

func onscreensFlagShowHandler(w http.ResponseWriter, r *http.Request) {
	base64content, ok := r.URL.Query()["duration"]
	if !ok || len(base64content) > 1 {
		http.Error(w, "417 expectation failed", http.StatusExpectationFailed)
		return
	}
	durStr, err := helpers.Base64Decode(base64content[0])
	if err != nil {
		terrors.Log(err, "unable to decode string")
		http.Error(w, "422 unprocessable entity", http.StatusUnprocessableEntity)
		return
	}
	dur, err := time.ParseDuration(durStr)
	if err != nil {
		http.Error(w, "unable to parse duration", http.StatusInternalServerError)
		return
	}
	onscreensServer.ShowFlag(dur)
	fmt.Fprintf(w, "OK")
}

func onscreensGpsHideHandler(w http.ResponseWriter, r *http.Request) {
	onscreensServer.HideGPSImage()
	fmt.Fprintf(w, "OK")
}

func onscreensGpsShowHandler(w http.ResponseWriter, r *http.Request) {
	onscreensServer.ShowGPSImage()
	fmt.Fprintf(w, "OK")
}

func onscreensTimewarpShowHandler(w http.ResponseWriter, r *http.Request) {
	onscreensServer.ShowTimewarp()
	fmt.Fprintf(w, "OK")
}

func onscreensLeaderboardShowHandler(w http.ResponseWriter, r *http.Request) {
	base64content, ok := r.URL.Query()["content"]
	if !ok || len(base64content) > 1 {
		http.Error(w, "417 expectation failed", http.StatusExpectationFailed)
		return
	}
	spew.Dump(base64content[0])
	content, err := helpers.Base64Decode(base64content[0])
	if err != nil {
		terrors.Log(err, "unable to decode string")
		http.Error(w, "422 unprocessable entity", http.StatusUnprocessableEntity)
		return
	}

	onscreensServer.Leaderboard.Show(content)
	fmt.Fprintf(w, "OK")
}

func onscreensMiddleHideHandler(w http.ResponseWriter, r *http.Request) {
	onscreensServer.MiddleText.Hide()
	fmt.Fprintf(w, "OK")

}

func onscreensMiddleShowHandler(w http.ResponseWriter, r *http.Request) {
	base64content, ok := r.URL.Query()["msg"]
	if !ok || len(base64content) > 1 {
		http.Error(w, "417 expectation failed", http.StatusExpectationFailed)
		return
	}
	msg, err := helpers.Base64Decode(base64content[0])
	if err != nil {
		terrors.Log(err, "unable to decode string")
		http.Error(w, "422 unprocessable entity", http.StatusUnprocessableEntity)
		return
	}
	onscreensServer.MiddleText.Show(msg)
	fmt.Fprintf(w, "OK")
}

func faviconHandler(w http.ResponseWriter, r *http.Request) {
	//	// return a favicon if anyone asks for one
	//} else if r.URL.Path == "/favicon.ico" {
	http.ServeFile(w, r, "assets/favicon.ico")
}

//TODO: use more StatusExpectationFailed instead of http.StatusUnprocessableEntity
func catchAllHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		http.Error(w, "404 not found", http.StatusNotFound)
		log.Println("someone tried hitting", r.URL.Path)
		return

	// someone tried a PUT or a DELETE or something
	default:
		fmt.Fprintf(w, "Only GET methods are supported.\n")
	}
}

// Start starts the web server
func Start() {
	log.Println("Starting VLC web server on host", config.VlcServerHost)

	r := mux.NewRouter()

	// add the prometheus middleware
	r.Use(helpers.PrometheusMiddleware)
	// make prometheus metrics available
	r.Path("/metrics").Handler(promhttp.Handler())

	// healthcheck endpoint
	//TODO: add /ready and /live
	r.HandleFunc("/health", healthHandler).Methods("GET")

	// vlc endpoints
	//TODO: consider refactoring into a subrouter
	r.HandleFunc("/vlc/current", vlcCurrentHandler).Methods("GET")
	r.HandleFunc("/vlc/play/{id:[0-9]+}", vlcPlayHandler).Methods("GET")
	r.HandleFunc("/vlc/back", vlcBackHandler).Methods("GET")
	r.HandleFunc("/vlc/skip", vlcSkipHandler).Methods("GET")
	r.HandleFunc("/vlc/random", vlcRandomHandler).Methods("GET")

	// onscreen endpoints
	//TODO: consider refactoring into a subrouter
	r.HandleFunc("/onscreens/flag/show", onscreensFlagShowHandler).Methods("GET")
	r.HandleFunc("/onscreens/gps/hide", onscreensGpsHideHandler).Methods("GET")
	r.HandleFunc("/onscreens/gps/show", onscreensGpsShowHandler).Methods("GET")
	r.HandleFunc("/onscreens/timewarp/show", onscreensTimewarpShowHandler).Methods("GET")
	r.HandleFunc("/onscreens/leaderboard/show", onscreensLeaderboardShowHandler).Methods("GET")
	r.HandleFunc("/onscreens/middle/hide", onscreensMiddleHideHandler).Methods("GET")
	r.HandleFunc("/onscreens/middle/show", onscreensMiddleShowHandler).Methods("GET")

	//TODO: refactor into static serving
	r.HandleFunc("/favicon.ico", faviconHandler).Methods("GET")

	//TODO: update to be proper catchall(?)
	// r.PathPrefix("/").Handler(catchAllHandler)
	r.HandleFunc("/", catchAllHandler)

	//TODO: error if there's no colon to split on
	port := strings.Split(config.VlcServerHost, ":")[1]
	addr := fmt.Sprintf("0.0.0.0:%s", port)

	srv := &http.Server{
		Addr: addr,
		// Good practice to set timeouts to avoid Slowloris attacks.
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      r, // Pass our instance of gorilla/mux in.
	}

	//TODO: add proper graceful shutdown
	//TODO: replace certs with autocert: https://stackoverflow.com/a/40494806
	// http.ListenAndServeTLS(port, "infra/fullchain.pem", "infra/private.key", nil)
	if err := srv.ListenAndServe(); err != nil {
		terrors.Fatal(err, "couldn't start server")
	}
}
