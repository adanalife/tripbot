package main

import (
	"database/sql"
	"math"
	"testing"
	"time"
)

var t0 = time.Date(2018, 5, 14, 22, 48, 1, 0, time.UTC)

// mkClip builds a usable OCR-fixed clip n 3-minute steps after t0.
func mkClip(id int, lat, lng float64, step int) clip {
	return clip{
		id:     id,
		slug:   "clip",
		lat:    lat,
		lng:    lng,
		source: sourceOCR,
		filmed: t0.Add(time.Duration(step) * 3 * time.Minute),
	}
}

func TestAnalyzeComputesChainDistances(t *testing.T) {
	// ~0.069 mi per 0.001° of latitude; both pairs well under 100 mph.
	clips := []clip{
		mkClip(1, 40.000, -111.000, 0),
		mkClip(2, 40.010, -111.000, 1),
		mkClip(3, 40.020, -111.000, 2),
	}
	out := analyze(clips, 5*time.Minute, 100)

	for _, i := range []int{0, 1} {
		if !out[i].newMiles.Valid {
			t.Fatalf("clip %d: expected a distance, got NULL (%s)", i, out[i].reason)
		}
		if math.Abs(out[i].newMiles.Float64-0.691) > 0.01 {
			t.Errorf("clip %d: distance = %v, want ~0.691", i, out[i].newMiles.Float64)
		}
	}
	if out[2].newMiles.Valid || out[2].reason != "last-fix" {
		t.Errorf("final clip should be NULL/last-fix, got %+v", out[2])
	}
}

func TestAnalyzeNullsRecordingGaps(t *testing.T) {
	clips := []clip{
		mkClip(1, 40.000, -111.000, 0),
		mkClip(2, 41.000, -111.000, 100), // 5 hours later — dashcam was off
		mkClip(3, 41.010, -111.000, 101),
	}
	out := analyze(clips, 5*time.Minute, 100)
	if out[0].newMiles.Valid || out[0].reason != "gap" {
		t.Errorf("gap pair should be NULL/gap, got %+v", out[0])
	}
	if !out[1].newMiles.Valid {
		t.Errorf("post-gap pair should still compute, got NULL (%s)", out[1].reason)
	}
}

func TestAnalyzeNullsImpossibleSpeeds(t *testing.T) {
	clips := []clip{
		mkClip(1, 40.000, -111.000, 0),
		mkClip(2, 42.000, -111.000, 1), // ~138 mi in 3 min
	}
	out := analyze(clips, 5*time.Minute, 100)
	if out[0].newMiles.Valid || out[0].reason != "speed" {
		t.Errorf("impossible-speed pair should be NULL/speed, got %+v", out[0])
	}
}

func TestAnalyzeSkipsUnusableFixesAndBridgesToNext(t *testing.T) {
	rejected := mkClip(2, 40.005, -111.000, 1)
	rejected.source = "rejected"
	clips := []clip{
		mkClip(1, 40.000, -111.000, 0),
		rejected,
		mkClip(3, 40.010, -111.000, 2), // 6 min from clip 1, inside the 10m gap budget
	}
	out := analyze(clips, 10*time.Minute, 100)
	if out[1].newMiles.Valid || out[1].reason != "no-fix" {
		t.Errorf("rejected clip should be NULL/no-fix, got %+v", out[1])
	}
	// clip 1 bridges over the rejected clip to clip 3
	if !out[0].newMiles.Valid {
		t.Fatalf("bridging pair should compute, got NULL (%s)", out[0].reason)
	}
	if math.Abs(out[0].newMiles.Float64-0.691) > 0.01 {
		t.Errorf("bridged distance = %v, want ~0.691", out[0].newMiles.Float64)
	}
}

func TestAnalyzeTreatsZeroCoordsAsMissing(t *testing.T) {
	zero := mkClip(1, 0, 0, 0)
	out := analyze([]clip{zero}, 5*time.Minute, 100)
	if out[0].newMiles.Valid || out[0].reason != "no-fix" {
		t.Errorf("0/0 clip should be NULL/no-fix, got %+v", out[0])
	}
}

func TestChangedIsIdempotentAcrossRealRoundTrip(t *testing.T) {
	c := mkClip(1, 40.000, -111.000, 0)
	// Simulate the stored REAL (float32) copy of a previously-written value.
	c.miles = sql.NullFloat64{Float64: float64(float32(0.691)), Valid: true}
	d := decision{clip: c, newMiles: sql.NullFloat64{Float64: 0.691, Valid: true}}
	if d.changed() {
		t.Error("re-running on an already-written value should not report a change")
	}

	// NULL → value and value → NULL are changes.
	d = decision{clip: mkClip(2, 40, -111, 0), newMiles: sql.NullFloat64{Float64: 0.5, Valid: true}}
	if !d.changed() {
		t.Error("NULL → value should report a change")
	}
	c2 := mkClip(3, 40, -111, 0)
	c2.miles = sql.NullFloat64{Float64: 0.5, Valid: true}
	if !(decision{clip: c2}).changed() {
		t.Error("value → NULL should report a change")
	}
}
