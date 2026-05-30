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
	var got [][2]float64
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body not valid JSON: %v\n%s", err, rec.Body.String())
	}
	if len(got) != 2 || got[0][0] != 41.5 || got[1][1] != -110.3 {
		t.Errorf("route = %v, want the two seeded points", got)
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
