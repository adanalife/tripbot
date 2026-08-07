package server

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/video"
)

// withCorpusRoute stages the route the handler reads, and clears the built
// response either side of the test. Without the reset a second test would be
// served the first one's cached body — the cache is process-wide by design, so
// staging a route is only meaningful alongside dropping it.
func withCorpusRoute(t *testing.T, pts [][2]float64) {
	t.Helper()
	withRoutePoints(t, trusted(pts))
}

// withRoutePoints is the banded form; withCorpusRoute is the shorthand for the
// common case where every point is a read one.
func withRoutePoints(t *testing.T, pts []video.RoutePoint) {
	t.Helper()
	saved := corpusRoute
	t.Cleanup(func() { corpusRoute = saved; resetCorpusCache() })
	corpusRoute = func(context.Context) []video.RoutePoint { return pts }
	resetCorpusCache()
}

// trusted tags plain coordinates as read ones, so a test that doesn't care
// about banding doesn't have to say so at every point.
func trusted(pts [][2]float64) []video.RoutePoint {
	out := make([]video.RoutePoint, len(pts))
	for i, p := range pts {
		out[i] = video.RoutePoint{Lat: p[0], Lng: p[1], Band: video.BandTrusted}
	}
	return out
}

func resetCorpusCache() {
	corpusCache.Lock()
	defer corpusCache.Unlock()
	corpusCache.body, corpusCache.builtAt = nil, time.Time{}
}

func TestMapCorpusHandler(t *testing.T) {
	withCorpusRoute(t, [][2]float64{{41.5, -110.2}, {41.6, -110.3}})

	rec := httptest.NewRecorder()
	mapCorpusHandler(rec, httptest.NewRequest(http.MethodGet, "/admin/map/corpus", nil))

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got []routeSegment
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body not valid JSON: %v\n%s", err, rec.Body.String())
	}
	// The two seeded points are ~9 miles apart (< the gap threshold) and share
	// a band, so they stay in a single segment.
	if len(got) != 1 || len(got[0].Points) != 2 ||
		got[0].Points[0][0] != 41.5 || got[0].Points[1][1] != -110.3 {
		t.Errorf("route = %v, want one segment of the two seeded points", got)
	}
	if got[0].Band != video.BandTrusted {
		t.Errorf("band = %q, want %q", got[0].Band, video.BandTrusted)
	}
}

// A bridged stretch has to arrive as its own segment, or the map cannot colour
// it: a Leaflet polyline is one colour, so the split is what carries the band.
// The boundary point belongs to both, so the two colours meet rather than
// leaving a hole in a line the van never stopped driving.
func TestSplitOnGaps_SplitsWhereTheBandChanges(t *testing.T) {
	pts := []video.RoutePoint{
		{Lat: 41.50, Lng: -110.20, Band: video.BandTrusted},
		{Lat: 41.51, Lng: -110.21, Band: video.BandTrusted},
		{Lat: 41.52, Lng: -110.22, Band: video.BandSynthetic},
		{Lat: 41.53, Lng: -110.23, Band: video.BandTrusted},
	}
	segs := splitOnGaps(pts, maxRouteGapMiles)

	if len(segs) != 3 {
		t.Fatalf("got %d segments, want 3: %v", len(segs), segs)
	}
	bands := []string{segs[0].Band, segs[1].Band, segs[2].Band}
	want := []string{video.BandTrusted, video.BandSynthetic, video.BandTrusted}
	for i := range want {
		if bands[i] != want[i] {
			t.Errorf("segment %d band = %q, want %q", i, bands[i], want[i])
		}
	}
	// Each boundary point is repeated, so consecutive segments share an end.
	if segs[0].Points[len(segs[0].Points)-1] != segs[1].Points[0] {
		t.Errorf("segments 0 and 1 do not meet: %v vs %v", segs[0].Points, segs[1].Points)
	}
	if segs[1].Points[len(segs[1].Points)-1] != segs[2].Points[0] {
		t.Errorf("segments 1 and 2 do not meet: %v vs %v", segs[1].Points, segs[2].Points)
	}
}

func TestSplitOnGaps(t *testing.T) {
	// Two clusters of nearby points separated by a cross-country jump should
	// split into two segments.
	pts := [][2]float64{
		{41.50, -110.20}, {41.51, -110.21}, // Wyoming
		{34.05, -118.24}, {34.06, -118.25}, // Los Angeles — ~700mi away
	}
	segs := splitOnGaps(trusted(pts), maxRouteGapMiles)
	if len(segs) != 2 {
		t.Fatalf("got %d segments, want 2: %v", len(segs), segs)
	}
	if len(segs[0].Points) != 2 || len(segs[1].Points) != 2 {
		t.Errorf("segment sizes = %d,%d, want 2,2", len(segs[0].Points), len(segs[1].Points))
	}

	// A contiguous run under the threshold stays a single segment.
	near := [][2]float64{{41.50, -110.20}, {41.55, -110.25}, {41.60, -110.30}}
	if segs := splitOnGaps(trusted(near), maxRouteGapMiles); len(segs) != 1 {
		t.Errorf("contiguous run split into %d segments, want 1", len(segs))
	}

	// Empty input returns a non-nil empty slice (encodes as [] not null).
	if segs := splitOnGaps(nil, maxRouteGapMiles); segs == nil || len(segs) != 0 {
		t.Errorf("empty input = %v, want non-nil empty slice", segs)
	}
}

func TestDownsample(t *testing.T) {
	short := [][2]float64{{1, 1}, {2, 2}}
	if out := downsample(short, 10); len(out) != 2 {
		t.Errorf("short input: len = %d, want 2 (unchanged)", len(out))
	}
	if out := downsample(short, 0); len(out) != 2 {
		t.Errorf("max<=0: should return input unchanged")
	}

	big := make([][2]float64, 1000)
	for i := range big {
		big[i] = [2]float64{float64(i), 0}
	}
	out := downsample(big, 100)
	if len(out) > 101 {
		t.Errorf("downsampled len = %d, want <= 101", len(out))
	}
	if out[len(out)-1] != big[len(big)-1] {
		t.Errorf("last point not preserved: got %v want %v", out[len(out)-1], big[len(big)-1])
	}
}

// The route is read once and reused: a click on the toggle shouldn't re-read a
// few hundred thousand track points from the DB.
func TestMapCorpusHandler_CachesTheBuiltRoute(t *testing.T) {
	reads := 0
	saved := corpusRoute
	t.Cleanup(func() { corpusRoute = saved; resetCorpusCache() })
	corpusRoute = func(context.Context) []video.RoutePoint {
		reads++
		return trusted([][2]float64{{41.5, -110.2}, {41.6, -110.3}})
	}
	resetCorpusCache()

	for range 3 {
		mapCorpusHandler(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/admin/map/corpus", nil))
	}
	if reads != 1 {
		t.Errorf("read the route %d times, want 1", reads)
	}
}

// A failed read must not be cached, or one DB blip pins an empty overlay for
// the whole TTL.
func TestMapCorpusHandler_EmptyRouteIsNotCached(t *testing.T) {
	reads := 0
	saved := corpusRoute
	t.Cleanup(func() { corpusRoute = saved; resetCorpusCache() })
	corpusRoute = func(context.Context) []video.RoutePoint { reads++; return nil }
	resetCorpusCache()

	rec := httptest.NewRecorder()
	mapCorpusHandler(rec, httptest.NewRequest(http.MethodGet, "/admin/map/corpus", nil))
	mapCorpusHandler(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/admin/map/corpus", nil))

	if reads != 2 {
		t.Errorf("read the route %d times, want 2 (a failed read isn't cached)", reads)
	}
	if body := rec.Body.String(); body != "[]" {
		t.Errorf("body = %q, want [] for an unavailable route", body)
	}
}

// simplify has to keep the shape and drop the filler: a corner survives, the
// points along a straight run don't, and the endpoints are never dropped.
func TestSimplify(t *testing.T) {
	// An L: east along a parallel, then north. The corner is the only interior
	// point that carries shape.
	corner := [][2]float64{{40.0, -105.0}, {40.0, -104.99}, {40.0, -104.98}, {40.01, -104.98}, {40.02, -104.98}}
	got := simplify(corner, 100)
	if len(got) != 3 {
		t.Errorf("simplified an L to %d points, want 3 (both ends + the corner): %v", len(got), got)
	}
	if got[0] != corner[0] || got[len(got)-1] != corner[len(corner)-1] {
		t.Errorf("endpoints changed: %v", got)
	}
	if len(got) == 3 && got[1] != corner[2] {
		t.Errorf("kept %v as the corner, want %v", got[1], corner[2])
	}

	// A dead-straight run collapses to its endpoints whatever its length.
	straight := make([][2]float64, 500)
	for i := range straight {
		straight[i] = [2]float64{40.0, -105.0 + float64(i)*0.001}
	}
	if got := simplify(straight, 100); len(got) != 2 {
		t.Errorf("simplified a straight line to %d points, want 2", len(got))
	}

	// A tighter bound keeps more of the same curve — the knob has to work in
	// the direction the comment claims.
	curve := make([][2]float64, 200)
	for i := range curve {
		f := float64(i) / 199
		curve[i] = [2]float64{40.0 + 0.05*f*f, -105.0 + 0.2*f}
	}
	loose, tight := simplify(curve, 250), simplify(curve, 25)
	if len(tight) <= len(loose) {
		t.Errorf("epsilon 25 kept %d points, epsilon 250 kept %d; tighter must keep more", len(tight), len(loose))
	}

	// Too short to have an interior point comes back untouched.
	for _, short := range [][][2]float64{nil, {{1, 2}}, {{1, 2}, {3, 4}}} {
		if got := simplify(short, 100); len(got) != len(short) {
			t.Errorf("simplify(%v) = %v, want it unchanged", short, got)
		}
	}
}

// Every kept point must be within epsilon of the line the simplified route
// draws — the guarantee the endpoint's payload budget is chosen against.
func TestSimplify_HonorsTheErrorBound(t *testing.T) {
	pts := make([][2]float64, 1000)
	for i := range pts {
		f := float64(i) / 999
		pts[i] = [2]float64{40.0 + 0.3*math.Sin(f*6), -105.0 + 0.5*f}
	}
	const eps = 100
	out := simplify(pts, eps)
	if len(out) >= len(pts) {
		t.Fatalf("simplify kept %d of %d points, expected a real reduction", len(out), len(pts))
	}

	// Walk the original, measuring each point against the simplified span it
	// falls inside.
	j, worst := 0, 0.0
	for _, p := range pts {
		if j+1 < len(out) && p == out[j+1] {
			j++
		}
		if j+1 >= len(out) {
			break
		}
		if d := perpMeters(p, out[j], out[j+1]); d > worst {
			worst = d
		}
	}
	if worst > eps {
		t.Errorf("a dropped point sits %.0f m off the simplified line, want <= %d m", worst, eps)
	}
}
