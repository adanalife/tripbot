package vlcServer

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/adanalife/tripbot/internal/config"
	terrors "github.com/adanalife/tripbot/internal/errors"
	onscreensServer "github.com/adanalife/tripbot/internal/onscreens-server"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/davecgh/go-spew/spew"
	"github.com/gorilla/mux"
)

// healthcheck URL, for tools to verify the stream is alive
func healthHandler(w http.ResponseWriter, r *http.Request) {
	if !helpers.RunningOnWindows() {
		obsPid := helpers.ReadPidFile(config.OBSPidFile)
		pidRunning, err := helpers.PidExists(obsPid)
		if err != nil {
			terrors.Log(err, "error fetching OBS pid")
			http.Error(w, "error fetching OBS pid", http.StatusFailedDependency)
			return
		}
		if !pidRunning {
			http.Error(w, "OBS not running", http.StatusFailedDependency)
			return
		}
	}
	fmt.Fprintf(w, "OK")
}

func vlcCurrentHandler(w http.ResponseWriter, r *http.Request) {
	// return the currently-playing file
	fmt.Fprintf(w, currentlyPlaying())
}

func vlcPlayHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	spew.Dump(vars)

	videoFile := vars["video"]

	spew.Dump(videoFile)
	playVideoFile(videoFile)

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

func onscreensFlagHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	spew.Dump(vars)

	switch vars["action"] {
	case "show":
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
	case "hide":
		onscreensServer.FlagImage.Hide()
		fmt.Fprintf(w, "OK")
	default:
		http.Error(w, "417 expectation failed", http.StatusExpectationFailed)
		return
	}
}

func onscreensGpsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	switch vars["action"] {
	case "show":
		onscreensServer.ShowGPSImage()
		fmt.Fprintf(w, "OK")
	case "hide":
		onscreensServer.HideGPSImage()
		fmt.Fprintf(w, "OK")
	default:
		http.Error(w, "417 expectation failed", http.StatusExpectationFailed)
		return
	}
}

func onscreensMiddleHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	switch vars["action"] {
	case "show":
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
	case "hide":
		onscreensServer.MiddleText.Hide()
		fmt.Fprintf(w, "OK")
	default:
		http.Error(w, "417 expectation failed", http.StatusExpectationFailed)
		return
	}
}

func onscreensTimewarpHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	switch vars["action"] {
	case "show":
		//TODO: is this different from Timewarp.Show()?
		onscreensServer.ShowTimewarp()
		fmt.Fprintf(w, "OK")
	case "hide":
		onscreensServer.Timewarp.Hide()
		fmt.Fprintf(w, "OK")
	default:
		http.Error(w, "417 expectation failed", http.StatusExpectationFailed)
		return
	}
}

func onscreensLeaderboardHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	switch vars["action"] {
	case "show":
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
	case "hide":
		onscreensServer.Leaderboard.Hide()
		fmt.Fprintf(w, "OK")
	default:
		http.Error(w, "417 expectation failed", http.StatusExpectationFailed)
		return
	}
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
		//TODO: theres an http error class for this
		fmt.Fprintf(w, "Only GET methods are supported.\n")
	}
}
