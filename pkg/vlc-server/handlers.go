package vlcServer

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"time"
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

// vlcCurrentHandler returns the currently-playing file. This is the only
// vlc command-surface route still on HTTP — it's a read. The play / random /
// skip / back commands moved to NATS (see nats.go).
func (s *Server) vlcCurrentHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, s.currentlyPlaying())
}

// nextFrameJPEGHandler serves the cached first-frame JPEG that the OBS
// cover layer renders during the inter-clip gap. 404 when the cache
// file is missing (e.g. vlc-server just started and the refresher
// hasn't run yet) so the browser source falls back to whatever it had.
// Cache-Control: no-store + a Last-Modified header lets CEF issue
// conditional GETs and the server answer 304 when nothing changed
// since the last 10s poll.
func (s *Server) nextFrameJPEGHandler(w http.ResponseWriter, r *http.Request) {
	path := NextFrameCachePath()
	st, err := os.Stat(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Last-Modified", st.ModTime().UTC().Format(http.TimeFormat))
	http.ServeFile(w, r, path)
}

// nextFrameHTMLHandler serves a tiny wrapper page that the OBS browser
// source loads. The page polls /next-frame.jpg every 10s with a
// cache-bust timestamp, so vlc-server doesn't need to push refresh
// signals via obs-websocket — the cover frame keeps itself current.
func (s *Server) nextFrameHTMLHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, nextFrameHTML)
}

const nextFrameHTML = `<!doctype html>
<html><head><meta charset="utf-8"><style>
html,body{margin:0;padding:0;height:100%;background:#000;overflow:hidden;}
img{display:block;width:100%;height:100%;object-fit:cover;}
</style></head><body>
<img id="f" src="/next-frame.jpg">
<script>
setInterval(function(){
  document.getElementById('f').src = '/next-frame.jpg?t=' + Date.now();
}, 10000);
</script>
</body></html>
`

func (s *Server) faviconHandler(w http.ResponseWriter, r *http.Request) {
	//	// return a favicon if anyone asks for one
	//} else if r.URL.Path == "/favicon.ico" {
	http.ServeFile(w, r, "assets/favicon.ico")
}

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
