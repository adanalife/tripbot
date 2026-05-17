package vlcServer

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"

	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/davecgh/go-spew/spew"
	"github.com/gorilla/mux"
)

// versionTag is set by main via SetVersion; overridden at build time
// through `-ldflags "-X main.version=..."`.
var versionTag = "dev"

// SetVersion lets cmd/vlc-server inject its build-time version string
// before the HTTP server starts.
func SetVersion(v string) {
	if v != "" {
		versionTag = v
	}
}

// healthcheck URL, for tools to verify the stream is alive
func healthHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "OK")
}

// versionHandler returns build metadata as JSON. The tag comes from the
// build-time ldflag; sha + built_at are read from the binary's embedded
// VCS info (Go's automatic -buildvcs).
func versionHandler(w http.ResponseWriter, r *http.Request) {
	resp := struct {
		Tag     string `json:"tag"`
		Sha     string `json:"sha"`
		BuiltAt string `json:"built_at"`
	}{Tag: versionTag}

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				resp.Sha = s.Value
			case "vcs.time":
				resp.BuiltAt = s.Value
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		terrors.Log(err, "couldn't encode version response")
	}
}

func vlcCurrentHandler(w http.ResponseWriter, r *http.Request) {
	// return the currently-playing file
	fmt.Fprint(w, currentlyPlaying())
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
		slog.InfoContext(r.Context(), "404 GET", "path", r.URL.Path)
		return

	// someone tried a PUT or a DELETE or something
	default:
		//TODO: there's an http error class for this
		fmt.Fprintf(w, "Only GET methods are supported.\n")
	}
}
