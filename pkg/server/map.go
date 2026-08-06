package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/adanalife/tripbot/pkg/video"
)

// corpusRoute is the data seam the map-overlay endpoint reads through,
// overridable in tests so the handler renders without a DB.
var corpusRoute = video.CorpusRoute

// corpusEpsilonMeters is how far the drawn line may stray from the real one.
// Simplification keeps whichever points are needed to hold that bound, so the
// budget is spent on curves and switchbacks and nothing is spent on the
// straight interstate runs — which is what an evenly-spaced sample gets
// backwards. Measured over the corpus: 100 m keeps 9,465 of 398,684 points
// (65 KB gzipped) with every real bend intact. Lower it for more fidelity;
// 50 m roughly doubles the point count.
const corpusEpsilonMeters = 100

// maxCorpusPoints is a backstop on the simplified polyline, not the thing that
// decides its shape — corpusEpsilonMeters does that, and lands well under this.
// It exists so a future corpus several times the size can't quietly turn into a
// multi-megabyte response; if it ever binds, raise the epsilon rather than this.
const maxCorpusPoints = 25000

// corpusTTL is how long a built response is reused. The corpus only changes
// when new footage is ingested, and the handler's own Cache-Control promises
// the same hour downstream.
const corpusTTL = time.Hour

// maxRouteGapMiles is the jump between consecutive clips above which the route
// is split into a new segment. The van often resumed recording hundreds of
// miles away (a new trip), and drawing a straight line across that gap is a
// rendering artifact, not real driving. Normal inter-clip spacing is a few
// miles, so this threshold cleanly separates trip boundaries from real travel.
const maxRouteGapMiles = 25

// corpusCache holds the built response so the route is read and simplified
// once an hour rather than once per click. Reading a few hundred thousand
// track points and simplifying them takes long enough to be worth not doing on
// a toggle, and the corpus only changes at ingest. Holding the encoded bytes
// (not the points) also keeps the JSON encode off the request path.
var corpusCache struct {
	sync.Mutex
	body []byte
	// builtAt is zero until the first successful build; a failed read isn't
	// cached, so a DB blip doesn't pin an empty route for an hour.
	builtAt time.Time
}

// corpusBody returns the encoded route, building it if the cache is cold or
// stale. Errors yield nil, which the handler serves as an empty route — the
// overlay is decoration, so it degrades rather than failing the page.
func corpusBody(ctx context.Context) []byte {
	corpusCache.Lock()
	defer corpusCache.Unlock()
	if corpusCache.body != nil && time.Since(corpusCache.builtAt) < corpusTTL {
		return corpusCache.body
	}
	pts := corpusRoute(ctx)
	if len(pts) == 0 {
		return nil
	}
	segs := splitOnGaps(pts, maxRouteGapMiles)
	for i, seg := range segs {
		segs[i] = downsample(simplify(seg, corpusEpsilonMeters), maxCorpusPoints)
	}
	body, err := json.Marshal(segs)
	if err != nil {
		slog.ErrorContext(ctx, "couldn't encode corpus route", "err", err)
		return nil
	}
	slog.InfoContext(ctx, "built corpus route",
		"raw_points", len(pts), "segments", len(segs), "bytes", len(body))
	corpusCache.body, corpusCache.builtAt = body, time.Now()
	return body
}

// mapCorpusHandler serves GET /admin/map/corpus: the full dashcam route as JSON
// [[[lat,lng],…],…] — a list of segments, broken wherever consecutive points
// jump more than maxRouteGapMiles (trip boundaries). Leaflet renders a nested
// array as a multi-polyline, so each segment draws as a disconnected line.
// Loaded lazily by the map's "show full route" toggle, cached an hour (the
// corpus rarely changes).
func mapCorpusHandler(w http.ResponseWriter, r *http.Request) {
	body := corpusBody(r.Context())
	if body == nil {
		body = []byte("[]")
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if _, err := w.Write(body); err != nil {
		slog.ErrorContext(r.Context(), "couldn't write corpus route", "err", err)
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

// simplify drops the points that don't change the shape of the line:
// Ramer–Douglas–Peucker, keeping every point that would otherwise sit more than
// epsilon metres from the simplified path. Endpoints are always kept, so a
// segment never shortens.
//
// This is the opposite trade to downsample. An evenly-spaced sample spends the
// same number of points on a straight interstate run as on a switchback, so
// raising its budget mostly buys more points on the straight parts; this spends
// them where the road actually bends.
func simplify(pts [][2]float64, epsilon float64) [][2]float64 {
	if len(pts) < 3 {
		return pts
	}
	keep := make([]bool, len(pts))
	keep[0], keep[len(pts)-1] = true, true

	// Explicit stack rather than recursion: the corpus is a few hundred
	// thousand points, and a pathological near-collinear run would recurse
	// once per point.
	type span struct{ i, j int }
	stack := []span{{0, len(pts) - 1}}
	for len(stack) > 0 {
		s := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if s.j <= s.i+1 {
			continue
		}
		worst, at := 0.0, s.i
		for k := s.i + 1; k < s.j; k++ {
			if d := perpMeters(pts[k], pts[s.i], pts[s.j]); d > worst {
				worst, at = d, k
			}
		}
		if worst > epsilon {
			keep[at] = true
			stack = append(stack, span{s.i, at}, span{at, s.j})
		}
	}

	out := make([][2]float64, 0, len(pts)/8)
	for i, k := range keep {
		if k {
			out = append(out, pts[i])
		}
	}
	return out
}

// perpMeters is the distance in metres from p to the segment a–b. Degrees are
// scaled to metres about a's latitude and treated as flat — over the
// kilometre-scale spans this is asked about, the error from ignoring curvature
// is far below the epsilon it's compared against.
func perpMeters(p, a, b [2]float64) float64 {
	const metersPerDegreeLat = 110540
	metersPerDegreeLng := 111320 * math.Cos(a[0]*math.Pi/180)

	px := (p[1] - a[1]) * metersPerDegreeLng
	py := (p[0] - a[0]) * metersPerDegreeLat
	bx := (b[1] - a[1]) * metersPerDegreeLng
	by := (b[0] - a[0]) * metersPerDegreeLat

	lenSq := bx*bx + by*by
	if lenSq == 0 {
		return math.Hypot(px, py)
	}
	// Clamped projection, so a point beyond either end measures to that end
	// rather than to the infinite line through them.
	t := (px*bx + py*by) / lenSq
	t = min(max(t, 0), 1)
	return math.Hypot(px-t*bx, py-t*by)
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
