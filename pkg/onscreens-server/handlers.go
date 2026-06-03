package onscreensServer

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/adanalife/tripbot/pkg/helpers"
	oe "github.com/adanalife/tripbot/pkg/onscreens-events"
	"github.com/gorilla/mux"
)

// onscreensFlagHandler is the HTTP twin of the flag.show / flag.hide NATS
// path (the client publishes over NATS; this stays for parity + manual
// debugging, e.g. curl '/onscreens/flag/show?state=MO'). show takes a
// two-letter state abbrev, which the asset handler resolves to the embedded
// per-state flag image.
func (s *Server) onscreensFlagHandler(w http.ResponseWriter, r *http.Request) {
	switch mux.Vars(r)["action"] {
	case "show":
		state := r.URL.Query().Get("state")
		if state == "" {
			http.Error(w, "417 expectation failed", http.StatusExpectationFailed)
			return
		}
		s.Flag.ShowFor(strings.ToUpper(state), flagDuration)
		fmt.Fprintf(w, "OK")
	case "hide":
		s.Flag.Hide()
		fmt.Fprintf(w, "OK")
	default:
		http.Error(w, "417 expectation failed", http.StatusExpectationFailed)
		return
	}
}

func (s *Server) onscreensGpsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	switch vars["action"] {
	case "show":
		s.GPS.Show("")
		fmt.Fprintf(w, "OK")
	case "hide":
		s.GPS.Hide()
		fmt.Fprintf(w, "OK")
	default:
		http.Error(w, "417 expectation failed", http.StatusExpectationFailed)
		return
	}
}

func (s *Server) onscreensMiddleHandler(w http.ResponseWriter, r *http.Request) {
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
			slog.ErrorContext(r.Context(), "unable to decode string", "err", err)
			http.Error(w, "422 unprocessable entity", http.StatusUnprocessableEntity)
			return
		}
		s.MiddleText.Show(msg)
		fmt.Fprintf(w, "OK")
	case "hide":
		s.MiddleText.Hide()
		fmt.Fprintf(w, "OK")
	default:
		http.Error(w, "417 expectation failed", http.StatusExpectationFailed)
		return
	}
}

func (s *Server) onscreensTimewarpHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	switch vars["action"] {
	case "show":
		s.Timewarp.ShowFor("Timewarp!", timewarpDuration)
		fmt.Fprintf(w, "OK")
	case "hide":
		s.Timewarp.Hide()
		fmt.Fprintf(w, "OK")
	default:
		http.Error(w, "417 expectation failed", http.StatusExpectationFailed)
		return
	}
}

func (s *Server) onscreensLeaderboardHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	switch vars["action"] {
	case "show":
		base64content, ok := r.URL.Query()["content"]
		if !ok || len(base64content) > 1 {
			http.Error(w, "417 expectation failed", http.StatusExpectationFailed)
			return
		}
		raw, err := helpers.Base64Decode(base64content[0])
		if err != nil {
			slog.ErrorContext(r.Context(), "unable to decode string", "err", err)
			http.Error(w, "422 unprocessable entity", http.StatusUnprocessableEntity)
			return
		}
		// content is base64(JSON {title, rows}); the server renders the HTML
		// (same path the NATS leaderboard.show handler takes).
		var ev oe.LeaderboardShow
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			slog.ErrorContext(r.Context(), "unable to decode leaderboard payload", "err", err)
			http.Error(w, "422 unprocessable entity", http.StatusUnprocessableEntity)
			return
		}

		s.Leaderboard.ShowFor(renderLeaderboard(ev.Title, ev.Rows), leaderboardDuration)
		fmt.Fprintf(w, "OK")
	case "hide":
		s.Leaderboard.Hide()
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
