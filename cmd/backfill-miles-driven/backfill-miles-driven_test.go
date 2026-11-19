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
		source: "ocr",
		speed:  1,
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
	out := analyze(clips, nil, 5*time.Minute, 100, 0.8)

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
	out := analyze(clips, nil, 5*time.Minute, 100, 0.8)
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
	out := analyze(clips, nil, 5*time.Minute, 100, 0.8)
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
	out := analyze(clips, nil, 10*time.Minute, 100, 0.8)
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
	out := analyze([]clip{zero}, nil, 5*time.Minute, 100, 0.8)
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

func TestByCorpusSplitsRuns(t *testing.T) {
	clips := []clip{{id: 1, corpus: "s1"}, {id: 2, corpus: "s1"}, {id: 3, corpus: "s2"}, {id: 4, corpus: "s2fast"}}
	runs := byCorpus(clips)
	if len(runs) != 3 || len(runs[0]) != 2 || runs[1][0].id != 3 || runs[2][0].id != 4 {
		t.Fatalf("byCorpus = %+v, want [s1 s1] [s2] [s2fast]", runs)
	}
	if byCorpus(nil) != nil {
		t.Error("byCorpus(nil) should be nil")
	}
}

// eastward builds a track of n fixes 2 s apart heading east along 40°N at
// 0.0005° of longitude a step: ~0.0265 mi per step, ~48 mph.
func eastward(n int) []fix {
	track := make([]fix, n)
	for i := range track {
		track[i] = fix{ts: float64(2 * i), lat: 40, lng: -111 + 0.0005*float64(i)}
	}
	return track
}

func TestTrackMilesSumsTheSteps(t *testing.T) {
	miles, ok := trackMiles(eastward(91), 1, 100)
	if !ok || math.Abs(miles-2.383) > 0.01 {
		t.Errorf("trackMiles = %v, %v; want ~2.383, true", miles, ok)
	}
	if _, ok := trackMiles(eastward(1), 1, 100); ok {
		t.Error("a one-fix track has no distance")
	}
}

// A misread costs nothing: the fixes either side of it still join up.
func TestTrackMilesStepsOverASpike(t *testing.T) {
	clean, _ := trackMiles(eastward(91), 1, 100)
	spiked := eastward(91)
	spiked[40].lat = 41 // ~69 mi north for one fix
	miles, ok := trackMiles(spiked, 1, 100)
	if !ok || math.Abs(miles-clean) > 0.001 {
		t.Errorf("spiked track = %v, %v; want %v, true", miles, ok, clean)
	}
}

// The same clip-seconds cover speed× the real time, so a time-lapse track may
// move speed× as far per step.
func TestTrackMilesScalesTheSpeedCapByClipSpeed(t *testing.T) {
	fast := eastward(91)
	for i := range fast {
		fast[i].lng = -111 + 0.003*float64(i) // ~286 mph at real time, ~48 at 6x
	}
	if _, ok := trackMiles(fast, 1, 100); ok {
		t.Error("at real time every step is too fast to believe")
	}
	if miles, ok := trackMiles(fast, 6, 100); !ok || miles < 14 {
		t.Errorf("at 6x = %v, %v; want ~14.3, true", miles, ok)
	}
}

func TestAnalyzePrefersAConfidentTrackAndFallsBackToTheChord(t *testing.T) {
	confident := mkClip(1, 40.000, -111.000, 0)
	confident.confidence = sql.NullFloat64{Float64: 0.95, Valid: true}
	doubtful := mkClip(2, 40.010, -111.000, 1)
	doubtful.confidence = sql.NullFloat64{Float64: 0.5, Valid: true}
	broken := mkClip(3, 40.020, -111.000, 2)
	broken.confidence = sql.NullFloat64{Float64: 0.95, Valid: true}
	last := mkClip(4, 40.030, -111.000, 3)

	// broken's track is believed by the coords stage but mostly unreachable.
	shattered := eastward(10)
	for i := 1; i < 9; i++ {
		shattered[i].lat = 45
	}
	tracks := map[int][]fix{1: eastward(91), 2: eastward(91), 3: shattered}
	out := analyze([]clip{confident, doubtful, broken, last}, tracks, 5*time.Minute, 100, 0.8)

	if out[0].newVia != viaTrack || math.Abs(out[0].newMiles.Float64-2.383) > 0.01 {
		t.Errorf("confident clip = %+v, want ~2.383 via track", out[0])
	}
	for _, i := range []int{1, 2} {
		if out[i].newVia != viaChord || math.Abs(out[i].newMiles.Float64-0.691) > 0.01 {
			t.Errorf("clip %d = %v via %q, want ~0.691 via chord", i, out[i].newMiles, out[i].newVia)
		}
	}
	if out[3].newMiles.Valid || out[3].newVia != "" {
		t.Errorf("final clip should be NULL with no source, got %+v", out[3])
	}
}

// A value whose source is not recorded yet is rewritten even when the number
// holds, so no row is left unlabeled.
func TestChangedWhenOnlyTheSourceDiffers(t *testing.T) {
	c := mkClip(1, 40, -111, 0)
	c.miles = sql.NullFloat64{Float64: 0.5, Valid: true}
	d := decision{clip: c, newMiles: sql.NullFloat64{Float64: 0.5, Valid: true}, newVia: viaChord}
	if !d.changed() {
		t.Error("an unlabeled value should report a change")
	}
	d.via = sql.NullString{String: viaChord, Valid: true}
	if d.changed() {
		t.Error("a labeled, equal value should not report a change")
	}
}
