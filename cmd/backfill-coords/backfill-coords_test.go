package main

import (
	"math"
	"testing"
	"time"
)

// base is an arbitrary film-start time; clips are spaced from here.
var base = time.Date(2018, 5, 12, 20, 0, 0, 0, time.UTC)

// at builds a clip filmed `sec` seconds after base. lat/lng of 0/0 means "no
// fix" (missing), matching the videos table convention.
func at(slug string, sec int, lat, lng float64) clip {
	return clip{slug: slug, lat: lat, lng: lng, filmed: base.Add(time.Duration(sec) * time.Second)}
}

func actions(ds []decision) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.action
	}
	return out
}

func eq(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("action[%d] = %q, want %q (got %v)", i, got[i], want[i], got)
		}
	}
}

const maxGap = 30 * time.Minute

// A clean straight-line track at ~30 mph (0.0001 deg ~= 36 ft; ~25ft/s) — every
// clip kept.
func TestAnalyze_CleanTrackAllKept(t *testing.T) {
	clips := []clip{
		at("a", 0, 40.0000, -111.0000),
		at("b", 5, 40.0001, -111.0000),
		at("c", 10, 40.0002, -111.0000),
		at("d", 15, 40.0003, -111.0000),
	}
	eq(t, actions(analyze(clips, 100, 2, 3, maxGap)),
		[]string{actionKeep, actionKeep, actionKeep, actionKeep})
}

// A single digit-flipped point (longitude off by ~1 degree, ~53 miles in 5s)
// is rejected, then interpolated back from its neighbours.
func TestAnalyze_DigitFlipRejectedThenInterpolated(t *testing.T) {
	clips := []clip{
		at("a", 0, 40.0000, -111.0000),
		at("b", 5, 40.0001, -110.0000), // OCR dropped a digit on lng
		at("c", 10, 40.0002, -111.0000),
	}
	ds := analyze(clips, 100, 2, 3, maxGap)
	eq(t, actions(ds), []string{actionKeep, actionInterpolate, actionKeep})

	got := ds[1]
	if got.newSource != sourceInterpolated {
		t.Fatalf("newSource = %q, want interpolated", got.newSource)
	}
	// midpoint in time → midpoint in space between a and c
	wantLat, wantLng := 40.0001, -111.0000
	if math.Abs(got.newLat-wantLat) > 1e-9 || math.Abs(got.newLng-wantLng) > 1e-9 {
		t.Fatalf("interpolated coord = %.6f,%.6f want %.6f,%.6f",
			got.newLat, got.newLng, wantLat, wantLng)
	}
}

// A missing point bracketed by good fixes within the gap budget is interpolated.
func TestAnalyze_MissingInterpolated(t *testing.T) {
	clips := []clip{
		at("a", 0, 40.0000, -111.0000),
		at("b", 5, 0, 0), // no GPS lock
		at("c", 10, 40.0002, -111.0000),
	}
	ds := analyze(clips, 100, 2, 3, maxGap)
	eq(t, actions(ds), []string{actionKeep, actionInterpolate, actionKeep})
}

// A missing point whose surrounding anchors are farther apart than the gap
// budget is left missing (van was parked/off — don't fabricate a midpoint).
func TestAnalyze_MissingBeyondGapLeftMissing(t *testing.T) {
	clips := []clip{
		at("a", 0, 40.0000, -111.0000),
		at("b", 3600, 0, 0), // 1h after a, 1h before c — both edges exceed 30m budget
		at("c", 7200, 40.0002, -111.0000),
	}
	ds := analyze(clips, 100, 2, 3, maxGap)
	eq(t, actions(ds), []string{actionKeep, actionMissing, actionKeep})
}

// Leading/trailing missing points have no anchor on one side and can't be
// interpolated.
func TestAnalyze_LeadingTrailingMissing(t *testing.T) {
	clips := []clip{
		at("a", 0, 0, 0), // leading: no anchor before
		at("b", 5, 40.0001, -111.0000),
		at("c", 10, 40.0002, -111.0000),
		at("d", 15, 0, 0), // trailing: no anchor after
	}
	ds := analyze(clips, 100, 2, 3, maxGap)
	eq(t, actions(ds),
		[]string{actionMissing, actionKeep, actionKeep, actionMissing})
}

// The recovery edge after a long parked gap is a single fast edge, not a
// there-and-back, so the resuming clip is kept (not falsely rejected).
func TestAnalyze_ParkedGapRecoveryNotRejected(t *testing.T) {
	clips := []clip{
		at("a", 0, 40.0000, -111.0000),
		// 10 minutes later, resumes 5 miles away — one fast edge in, slow out.
		at("b", 600, 40.0700, -111.0000),
		at("c", 605, 40.0701, -111.0000),
	}
	ds := analyze(clips, 100, 2, 3, maxGap)
	eq(t, actions(ds), []string{actionKeep, actionKeep, actionKeep})
}

// Two eastward spikes where the smaller one (earlier in film order) is masked
// on the first detection pass — its detour looks normal only because its
// neighbour is also displaced. Once the bigger spike is corrected, the smaller
// one is exposed and caught on a later pass. Verifies analyze converges rather
// than needing repeated runs.
func TestAnalyze_ConvergesOnMaskedOutlier(t *testing.T) {
	clips := []clip{
		at("a", 0, 40.0, -111.0),    // on track
		at("b", 600, 40.0, -110.5),  // ~26mi east — masked on pass 1
		at("c", 1200, 40.0, -110.0), // ~53mi east — caught on pass 1
		at("d", 1800, 40.0, -111.0), // on track
	}
	ds := analyze(clips, 100, 2, 2, maxGap)
	eq(t, actions(ds),
		[]string{actionKeep, actionInterpolate, actionInterpolate, actionKeep})
	if !ds[1].wasOutlier || !ds[2].wasOutlier {
		t.Fatalf("both spikes should be flagged as outliers: b=%v c=%v", ds[1].wasOutlier, ds[2].wasOutlier)
	}
}

// A run of two consecutive missing points between anchors is each interpolated.
func TestAnalyze_ConsecutiveMissingRun(t *testing.T) {
	clips := []clip{
		at("a", 0, 40.0000, -111.0000),
		at("b", 5, 0, 0),
		at("c", 10, 0, 0),
		at("d", 15, 40.0003, -111.0000),
	}
	ds := analyze(clips, 100, 2, 3, maxGap)
	eq(t, actions(ds),
		[]string{actionKeep, actionInterpolate, actionInterpolate, actionKeep})
	// b is 1/3 of the way from a to d in time → 1/3 in space
	if math.Abs(ds[1].newLat-40.0001) > 1e-9 {
		t.Fatalf("b lat = %.6f, want ~40.0001", ds[1].newLat)
	}
}
