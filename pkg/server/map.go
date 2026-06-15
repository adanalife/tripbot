package server

import (
	"encoding/json"
	"log/slog"
	"math"
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

// maxRouteGapMiles is the jump between consecutive clips above which the route
// is split into a new segment. The van often resumed recording hundreds of
// miles away (a new trip), and drawing a straight line across that gap is a
// rendering artifact, not real driving. Normal inter-clip spacing is a few
// miles, so this threshold cleanly separates trip boundaries from real travel.
const maxRouteGapMiles = 25

// mapCorpusHandler serves GET /admin/map/corpus: the full dashcam route as JSON
// [[[lat,lng],…],…] — a list of segments, broken wherever consecutive clips
// jump more than maxRouteGapMiles (trip boundaries). Leaflet renders a nested
// array as a multi-polyline, so each segment draws as a disconnected line.
// Loaded lazily by the map's "show full route" toggle, cached an hour (the
// corpus rarely changes).
func mapCorpusHandler(w http.ResponseWriter, r *http.Request) {
	segs := splitOnGaps(downsample(corpusRoute(r.Context()), maxCorpusPoints), maxRouteGapMiles)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if err := json.NewEncoder(w).Encode(segs); err != nil {
		slog.ErrorContext(r.Context(), "couldn't encode corpus route", "err", err)
	}
}

// splitOnGaps breaks an ordered point list into contiguous segments, starting a
// new segment whenever the great-circle distance between consecutive points
// exceeds maxMiles. Returns an empty (non-nil) slice for empty input so the JSON
// is [] rather than null.
func splitOnGaps(pts [][2]float64, maxMiles float64) [][][2]float64 {
	segs := make([][][2]float64, 0)
	if len(pts) == 0 {
		return segs
	}
	cur := [][2]float64{pts[0]}
	for i := 1; i < len(pts); i++ {
		prev, p := pts[i-1], pts[i]
		if milesBetween(prev[0], prev[1], p[0], p[1]) > maxMiles {
			segs = append(segs, cur)
			cur = [][2]float64{p}
			continue
		}
		cur = append(cur, p)
	}
	return append(segs, cur)
}

// milesBetween returns the great-circle (haversine) distance in miles between
// two lat/lng points.
func milesBetween(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusMiles = 3958.8
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLng := (lng2 - lng1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return earthRadiusMiles * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
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
