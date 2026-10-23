package server

import (
	"log/slog"
	"net/http"

	"github.com/adanalife/tripbot/pkg/feature"
	"github.com/adanalife/tripbot/pkg/httpmw"
)

// SetVersion lets cmd/tripbot inject its build-time version string
// before the HTTP server starts.
func (s *Server) SetVersion(v string) {
	if v != "" {
		s.versionTag = v
	}
}

// SetFlags injects the feature-flag client backing the console's /api/flags
// endpoints. Called from cmd/tripbot's startFeatureFlags, after Start has the
// HTTP server answering, so it takes lateMu. Left nil when the Postgres client
// fails to load — /api/flags then reports ok=false.
func (s *Server) SetFlags(fc feature.FlagClient) {
	s.lateMu.Lock()
	defer s.lateMu.Unlock()
	s.flags = fc
}

// SetBeds injects the background-audio bed store backing /api/audio. Called
// from cmd/tripbot after Start has the HTTP server answering, so it takes
// lateMu. Left nil on a tripbot with no OBS pairing — /api/audio then reports
// ok=false and the console shows the panel unavailable.
func (s *Server) SetBeds(b BedStore) {
	s.lateMu.Lock()
	defer s.lateMu.Unlock()
	s.beds = b
}

// versionHandler returns build metadata as JSON; see httpmw.WriteVersion.
// The tag comes from the build-time ldflag, injected through SetVersion.
func (s *Server) versionHandler(w http.ResponseWriter, r *http.Request) {
	httpmw.WriteVersion(w, r, s.versionTag)
}

// return a favicon if anyone asks for one
func faviconHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "assets/favicon.ico")
}

// catchAllHandler is the router's NotFoundHandler: every request reaching it
// named a path no route serves, whatever its method (a known path with the
// wrong method gets mux's own 405), so it answers 404.
func catchAllHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "404 not found", http.StatusNotFound)
	slog.InfoContext(r.Context(), "404 "+r.Method, "path", r.URL.Path)
}
