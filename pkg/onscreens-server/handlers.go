package onscreensServer

import (
	"fmt"
	"log/slog"
	"net/http"

	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/davecgh/go-spew/spew"
	"github.com/gorilla/mux"
)

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
		//TODO: fix this
		http.Error(w, "501 not implemented", http.StatusNotImplemented)
		return
		//durStr, err := helpers.Base64Decode(base64content[0])
		//if err != nil {
		//	terrors.Log(err, "unable to decode string")
		//	http.Error(w, "422 unable to decode string", http.StatusUnprocessableEntity)
		//	return
		//}
		//dur, err := time.ParseDuration(durStr)
		//if err != nil {
		//	http.Error(w, "422 unable to parse duration", http.StatusUnprocessableEntity)
		//	return
		//}
		//ShowFlag(dur)
		//fmt.Fprintf(w, "OK")
	case "hide":
		Lookup(SlugFlag).Hide()
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
		ShowGPSImage()
		fmt.Fprintf(w, "OK")
	case "hide":
		HideGPSImage()
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
		Lookup(SlugMiddleText).Show(msg)
		fmt.Fprintf(w, "OK")
	case "hide":
		Lookup(SlugMiddleText).Hide()
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
		ShowTimewarp()
		fmt.Fprintf(w, "OK")
	case "hide":
		Lookup(SlugTimewarp).Hide()
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
		content, err := helpers.Base64Decode(base64content[0])
		if err != nil {
			terrors.Log(err, "unable to decode string")
			http.Error(w, "422 unprocessable entity", http.StatusUnprocessableEntity)
			return
		}

		ShowLeaderboard(content)
		fmt.Fprintf(w, "OK")
	case "hide":
		Lookup(SlugLeaderboard).Hide()
		fmt.Fprintf(w, "OK")
	default:
		http.Error(w, "417 expectation failed", http.StatusExpectationFailed)
		return
	}
}

// healthHandler is the liveness probe target.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "OK")
}

// catchAllHandler 404s unknown routes for visibility.
func catchAllHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		http.Error(w, "404 not found", http.StatusNotFound)
		slog.InfoContext(r.Context(), "404 GET", "path", r.URL.Path)
		return
	default:
		fmt.Fprintf(w, "Only GET methods are supported.\n")
	}
}
