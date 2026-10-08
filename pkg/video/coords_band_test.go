package video

import "testing"

// The bands come off the two thresholds the pipeline already writes against,
// so the boundaries are the interesting cases: 0.8 is the trusted gate every
// consumer applies, and 0.5 is exactly what synth stamps on a bridged clip.
func TestRouteBand(t *testing.T) {
	conf := func(f float64) *float64 { return &f }

	for _, tc := range []struct {
		name string
		in   *float64
		want string
	}{
		{"a read track", conf(1.0), BandTrusted},
		{"at the trusted gate", conf(minCoordConfidence), BandTrusted},
		{"just under it", conf(0.79), BandSynthetic},
		{"a bridged clip", conf(SynthConfidence), BandSynthetic},
		{"the long-gap tier", conf(0.4), BandUntrusted},
		{"a refused track", conf(0.0), BandUntrusted},
		{"never measured", nil, BandUntrusted},
	} {
		if got := RouteBand(tc.in); got != tc.want {
			t.Errorf("%s: RouteBand() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// A clip the map draws per-moment must be one CoordAt would still refuse to
// answer from: the map wants the road's shape, which a bridge has, and !location
// wants a position, which it does not.
func TestRouteFloorIsBelowTheAnswerFloor(t *testing.T) {
	if minRouteConfidence >= minCoordConfidence {
		t.Fatalf("minRouteConfidence %v should sit below minCoordConfidence %v",
			minRouteConfidence, minCoordConfidence)
	}
}
