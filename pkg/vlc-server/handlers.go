package vlcServer

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/gorilla/mux"
)

// livenessHandler answers /health/ and /health/live. Liveness is a
// process-is-alive signal — if this handler runs at all, the answer is
// yes. Don't consult libvlc here; deeper checks belong on /health/ready
// (a stuck player should fail readiness, not get the pod restarted).
func (s *Server) livenessHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "OK")
}

// readinessHandler answers /health/ready by consulting Server.Health(),
// which reflects libvlc player state. Returns 503 with the error message
// when the player isn't in a state that can serve a viewer, so K8s
// readiness probes will pull the pod out of rotation while the player
// recovers (vs. liveness, which would restart the process).
func (s *Server) readinessHandler(w http.ResponseWriter, r *http.Request) {
	if err := s.Health(); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	fmt.Fprintf(w, "OK")
}

// rtspHealthHandler answers /health/rtsp by sending a local RTSP DESCRIBE
// against the libvlc listener. Returns 200 OK on a 200 response, 503 with
// the failure detail otherwise. Surfaces the same signal the self-heal
// watchdog uses so operators can probe it manually without waiting on the
// failure threshold.
func (s *Server) rtspHealthHandler(w http.ResponseWriter, r *http.Request) {
	if err := probeRTSPDescribe(); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	fmt.Fprintf(w, "OK")
}

// versionHandler returns build metadata as JSON. The tag comes from
// Server.Version (injected via Config at construction time); sha +
// built_at are read from the binary's embedded VCS info (Go's automatic
// -buildvcs). started_at is when the process began so callers can derive
// uptime themselves.
func (s *Server) versionHandler(w http.ResponseWriter, r *http.Request) {
	tag := s.Version
	if tag == "" {
		tag = "dev"
	}
	resp := struct {
		Tag       string `json:"tag"`
		Sha       string `json:"sha"`
		BuiltAt   string `json:"built_at"`
		StartedAt string `json:"started_at"`
	}{Tag: tag, StartedAt: startedAt.UTC().Format(time.RFC3339)}

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, st := range info.Settings {
			switch st.Key {
			case "vcs.revision":
				resp.Sha = st.Value
			case "vcs.time":
				resp.BuiltAt = st.Value
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.ErrorContext(r.Context(), "couldn't encode version response", "err", err)
	}
}

// startedAt marks process start so /version can report uptime. Set at
// package load; close enough to process start for a human-readable "up Xh".
var startedAt = time.Now()

func (s *Server) vlcCurrentHandler(w http.ResponseWriter, r *http.Request) {
	// return the currently-playing file
	fmt.Fprint(w, s.currentlyPlaying())
}

func (s *Server) vlcPlayHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	spew.Dump(vars)

	videoFile := vars["video"]

	spew.Dump(videoFile)
	if err := s.PlayVideoFile(videoFile); err != nil {
		slog.ErrorContext(r.Context(), "couldn't play requested video", "err", err, "video", videoFile)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	//TODO: better response
	fmt.Fprintf(w, "OK")
}

func (s *Server) vlcBackHandler(w http.ResponseWriter, r *http.Request) {
	num, ok := r.URL.Query()["n"]
	if !ok || len(num) > 1 {
		s.back(1)
		return
	}
	i, err := strconv.Atoi(num[0])
	if err != nil {
		slog.ErrorContext(r.Context(), "couldn't convert input to int", "err", err)
		http.Error(w, "422 unprocessable entity", http.StatusUnprocessableEntity)
		return
	}

	s.back(i)

	//TODO: better response
	fmt.Fprintf(w, "OK")

}

func (s *Server) vlcSkipHandler(w http.ResponseWriter, r *http.Request) {
	num, ok := r.URL.Query()["n"]
	if !ok || len(num) > 1 {
		s.skip(1)
		return
	}
	i, err := strconv.Atoi(num[0])
	if err != nil {
		slog.ErrorContext(r.Context(), "couldn't convert input to int", "err", err)
		http.Error(w, "422 unprocessable entity", http.StatusUnprocessableEntity)
		return
	}

	s.skip(i)

	//TODO: better response
	fmt.Fprintf(w, "OK")
}

func (s *Server) vlcRandomHandler(w http.ResponseWriter, r *http.Request) {
	// play a random file
	err := s.PlayRandom()
	if err != nil {
		http.Error(w, "error playing random", http.StatusInternalServerError)
	}
	fmt.Fprintf(w, "OK")
}

func (s *Server) faviconHandler(w http.ResponseWriter, r *http.Request) {
	//	// return a favicon if anyone asks for one
	//} else if r.URL.Path == "/favicon.ico" {
	http.ServeFile(w, r, "assets/favicon.ico")
}

//TODO: use more StatusExpectationFailed instead of http.StatusUnprocessableEntity
func (s *Server) catchAllHandler(w http.ResponseWriter, r *http.Request) {
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
