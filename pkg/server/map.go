package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/adanalife/tripbot/pkg/video"
)

// corpusRoute is the data seam the map-overlay endpoint reads through,
// overridable in tests so the handler renders without a DB.
var corpusRoute = video.CorpusRoute

// maxCorpusPoints caps the corpus polyline. It's a faint background route, so a
// few thousand points is ample detail — and keeps the JSON + the Leaflet path
// light. Larger corpora are evenly downsampled.
const maxCorpusPoints = 2500

// mapCorpusHandler serves GET /admin/map/corpus: the full dashcam route as JSON
// [[lat,lng],…]. Loaded lazily by the map's "show full route" toggle, cached an
// hour (the corpus rarely changes).
func mapCorpusHandler(w http.ResponseWriter, r *http.Request) {
	pts := downsample(corpusRoute(r.Context()), maxCorpusPoints)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if err := json.NewEncoder(w).Encode(pts); err != nil {
		slog.ErrorContext(r.Context(), "couldn't encode corpus route", "err", err)
	}
}

// downsample returns at most max evenly-spaced points, always keeping the last
// one so the route reaches its end. max <= 0 or a short input is returned as-is.
func downsample(pts [][2]float64, max int) [][2]float64 {
	if max <= 0 || len(pts) <= max {
		return pts
	}
	step := (len(pts) + max - 1) / max
	out := make([][2]float64, 0, max+1)
	for i := 0; i < len(pts); i += step {
		out = append(out, pts[i])
	}
	if last := pts[len(pts)-1]; out[len(out)-1] != last {
		out = append(out, last)
	}
	return out
}
