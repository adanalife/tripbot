package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func withCorpusRoute(t *testing.T, pts [][2]float64) {
	t.Helper()
	saved := corpusRoute
	t.Cleanup(func() { corpusRoute = saved })
	corpusRoute = func(context.Context) [][2]float64 { return pts }
}

func TestMapCorpusHandler(t *testing.T) {
	withCorpusRoute(t, [][2]float64{{41.5, -110.2}, {41.6, -110.3}})

	rec := httptest.NewRecorder()
	mapCorpusHandler(rec, httptest.NewRequest(http.MethodGet, "/admin/map/corpus", nil))

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got [][][2]float64
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body not valid JSON: %v\n%s", err, rec.Body.String())
	}
	// The two seeded points are ~9 miles apart (< the gap threshold), so they
	// stay in a single segment.
	if len(got) != 1 || len(got[0]) != 2 || got[0][0][0] != 41.5 || got[0][1][1] != -110.3 {
		t.Errorf("route = %v, want one segment of the two seeded points", got)
	}
}

func TestSplitOnGaps(t *testing.T) {
	// Two clusters of nearby points separated by a cross-country jump should
	// split into two segments.
	pts := [][2]float64{
		{41.50, -110.20}, {41.51, -110.21}, // Wyoming
		{34.05, -118.24}, {34.06, -118.25}, // Los Angeles — ~700mi away
	}
	segs := splitOnGaps(pts, maxRouteGapMiles)
	if len(segs) != 2 {
		t.Fatalf("got %d segments, want 2: %v", len(segs), segs)
	}
	if len(segs[0]) != 2 || len(segs[1]) != 2 {
		t.Errorf("segment sizes = %d,%d, want 2,2", len(segs[0]), len(segs[1]))
	}

	// A contiguous run under the threshold stays a single segment.
	near := [][2]float64{{41.50, -110.20}, {41.55, -110.25}, {41.60, -110.30}}
	if segs := splitOnGaps(near, maxRouteGapMiles); len(segs) != 1 {
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
