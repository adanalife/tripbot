package audiowatchdog

import (
	"math"
	"testing"
)

func TestPeakDB(t *testing.T) {
	cases := []struct {
		name   string
		levels [][3]float64
		want   float64
	}{
		{"no channels is silence", nil, silenceFloorDB},
		{"zero multiplier is silence", [][3]float64{{0, 0, 0}}, silenceFloorDB},
		{"unity peak is 0 dB", [][3]float64{{0.5, 1.0, 1.0}}, 0},
		{"half peak is ~-6 dB", [][3]float64{{0.1, 0.5, 0.5}}, -6.0206},
		{"takes the loudest channel's peak", [][3]float64{{0, 0.25, 0.25}, {0, 1.0, 1.0}}, 0},
		{"sub-floor multiplier clamps to floor", [][3]float64{{0, 0.0001, 0.0001}}, silenceFloorDB},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := peakDB(c.levels)
			if math.Abs(got-c.want) > 0.01 {
				t.Fatalf("peakDB(%v) = %v, want %v", c.levels, got, c.want)
			}
		})
	}
}
