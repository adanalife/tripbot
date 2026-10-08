package server

import (
	"encoding/binary"
	"math"
	"slices"
	"testing"

	"github.com/adanalife/tripbot/pkg/video"
)

// fuzzBands are the values RouteBand can produce, plus an empty one for a
// point whose band never got set.
var fuzzBands = []string{video.BandTrusted, video.BandSynthetic, video.BandUntrusted, ""}

// routePointsFromBytes decodes the fuzz corpus into route points, 17 bytes
// each: two float64 coordinates and a band selector. Arbitrary bit patterns
// mean NaN and ±Inf coordinates arrive for free, which is the interesting
// edge for the great-circle distance the splitter keys on.
func routePointsFromBytes(b []byte) []video.RoutePoint {
	const pointSize = 17
	pts := make([]video.RoutePoint, 0, len(b)/pointSize)
	for len(b) >= pointSize {
		pts = append(pts, video.RoutePoint{
			Lat:  math.Float64frombits(binary.LittleEndian.Uint64(b[0:8])),
			Lng:  math.Float64frombits(binary.LittleEndian.Uint64(b[8:16])),
			Band: fuzzBands[int(b[16])%len(fuzzBands)],
		})
		b = b[pointSize:]
	}
	return pts
}

// samePoint compares by bit pattern so a NaN coordinate carried through the
// splitter still matches the input point it came from.
func samePoint(a [2]float64, p video.RoutePoint) bool {
	return math.Float64bits(a[0]) == math.Float64bits(p.Lat) &&
		math.Float64bits(a[1]) == math.Float64bits(p.Lng)
}

// FuzzSplitOnGaps feeds the route splitter arbitrary geometry — NaN and
// infinite coordinates, degenerate gap thresholds, single points, no points.
// The invariants: it never panics, it never returns a nil or empty segment,
// every segment's band is the band of the point it opens with, and the emitted
// points are the input points in order, with only the boundary point a band
// change deliberately repeats.
func FuzzSplitOnGaps(f *testing.F) {
	// A short run of real-ish coordinates, then the shapes that break naive
	// float geometry.
	f.Add([]byte{}, float64(maxRouteGapMiles))
	f.Add(pointBytes(41.50, -110.20, 0), float64(maxRouteGapMiles))
	f.Add(append(pointBytes(41.50, -110.20, 0), pointBytes(41.51, -110.21, 1)...), float64(maxRouteGapMiles))
	f.Add(append(pointBytes(41.50, -110.20, 0), pointBytes(0, 0, 0)...), float64(maxRouteGapMiles))
	f.Add(append(pointBytes(math.NaN(), math.NaN(), 0), pointBytes(41.51, -110.21, 2)...), float64(maxRouteGapMiles))
	f.Add(append(pointBytes(math.Inf(1), math.Inf(-1), 3), pointBytes(0, 0, 0)...), 0.0)
	f.Add(pointBytes(41.50, -110.20, 0), math.NaN())
	f.Add(pointBytes(41.50, -110.20, 0), -1.0)

	f.Fuzz(func(t *testing.T, b []byte, maxMiles float64) {
		pts := routePointsFromBytes(b)
		segs := splitOnGaps(pts, maxMiles)

		if segs == nil {
			t.Fatal("splitOnGaps returned nil, want an empty slice so the JSON is []")
		}
		if len(pts) == 0 {
			if len(segs) != 0 {
				t.Fatalf("got %d segments for no points, want none", len(segs))
			}
			return
		}

		// Every emitted point has to be one of the input points.
		for si, seg := range segs {
			if len(seg.Points) == 0 {
				t.Fatalf("segment %d is empty", si)
			}
			for _, p := range seg.Points {
				if !slices.ContainsFunc(pts, func(q video.RoutePoint) bool { return samePoint(p, q) }) {
					t.Fatalf("segment %d emitted %v, which is not an input point", si, p)
				}
			}
		}

		// The order walk below reads a repeated point as the boundary one a
		// band change carries into both segments, which is only unambiguous
		// when no two consecutive input points are identical.
		for i := 1; i < len(pts); i++ {
			if samePoint([2]float64{pts[i].Lat, pts[i].Lng}, pts[i-1]) {
				return
			}
		}

		// Walk the emitted points against the input. Each one either consumes
		// the next input point or repeats the one just consumed (the boundary
		// point a band change carries into both segments); nothing else is
		// allowed, and every input point has to be consumed.
		next := 0
		for si, seg := range segs {
			for pi, p := range seg.Points {
				switch {
				case next < len(pts) && samePoint(p, pts[next]):
					if pi == 0 && seg.Band != pts[next].Band {
						t.Fatalf("segment %d band %q does not match its first point's band %q",
							si, seg.Band, pts[next].Band)
					}
					next++
				case next > 0 && samePoint(p, pts[next-1]):
					// The boundary point a band change carries into both
					// segments: it closes the old segment under the old
					// band and opens the new one under its own.
					if pi == 0 && seg.Band != pts[next-1].Band {
						t.Fatalf("segment %d band %q does not match the carried point's band %q",
							si, seg.Band, pts[next-1].Band)
					}
				default:
					t.Fatalf("segment %d point %d %v is not the next input point (index %d of %d)",
						si, pi, p, next, len(pts))
				}
			}
		}
		if next != len(pts) {
			t.Fatalf("consumed %d of %d input points", next, len(pts))
		}
	})
}

// pointBytes encodes one seed point in the layout routePointsFromBytes reads.
func pointBytes(lat, lng float64, band byte) []byte {
	b := make([]byte, 17)
	binary.LittleEndian.PutUint64(b[0:8], math.Float64bits(lat))
	binary.LittleEndian.PutUint64(b[8:16], math.Float64bits(lng))
	b[16] = band
	return b
}
