package video

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/database/testdb"
)

func TestCompass(t *testing.T) {
	tests := []struct {
		bearing float64
		want    string
	}{
		{0, "north"},
		{22.4, "north"}, // just inside north's upper half
		{22.6, "northeast"},
		{337.6, "north"}, // just inside north's lower half
		{337.4, "northwest"},
		{45, "northeast"},
		{90, "east"},
		{135, "southeast"},
		{180, "south"},
		{225, "southwest"},
		{270, "west"},
		{315, "northwest"},
		{359.9, "north"},
	}
	for _, tc := range tests {
		if got := Compass(tc.bearing); got != tc.want {
			t.Errorf("Compass(%v) = %q, want %q", tc.bearing, got, tc.want)
		}
	}
}

func TestCompassAbbrev(t *testing.T) {
	for bearing, want := range map[float64]string{
		0: "N", 45: "NE", 90: "E", 135: "SE", 180: "S", 225: "SW", 270: "W", 315: "NW", 337.6: "N",
	} {
		if got := CompassAbbrev(bearing); got != want {
			t.Errorf("CompassAbbrev(%v) = %q, want %q", bearing, got, want)
		}
	}
}

// Compass must name a point for every bearing a track can produce — an
// off-by-one in the wrap-around arithmetic would panic on the index rather
// than return a wrong word, so this asserts total coverage of the circle.
func TestCompassCoversTheCircle(t *testing.T) {
	for deg := 0.0; deg < 360; deg += 0.5 {
		if Compass(deg) == "" {
			t.Fatalf("Compass(%v) named nothing", deg)
		}
	}
}

func TestBearing(t *testing.T) {
	// From a point on the equator, one degree along each axis is very nearly
	// a cardinal bearing.
	origin := Moment{Lat: 0, Lng: 0}
	tests := []struct {
		name string
		to   Moment
		want float64
	}{
		{"due north", Moment{Lat: 1, Lng: 0}, 0},
		{"due east", Moment{Lat: 0, Lng: 1}, 90},
		{"due south", Moment{Lat: -1, Lng: 0}, 180},
		{"due west", Moment{Lat: 0, Lng: -1}, 270},
	}
	for _, tc := range tests {
		got := Bearing(origin, tc.to)
		if math.Abs(got-tc.want) > 0.5 {
			t.Errorf("%s: Bearing = %v, want ~%v", tc.name, got, tc.want)
		}
	}
}

// A northeast leg must not read as north: the whole point of the bearing is
// that it distinguishes the diagonals, which a sign-only implementation would
// not.
func TestBearingDiagonal(t *testing.T) {
	got := Bearing(Moment{Lat: 40, Lng: -100}, Moment{Lat: 40.01, Lng: -99.99})
	if Compass(got) != "northeast" {
		t.Errorf("Bearing = %v (%s), want northeast", got, Compass(got))
	}
}

func TestDistanceM(t *testing.T) {
	// One degree of latitude is ~111 km anywhere on Earth.
	got := distanceM(Moment{Lat: 40, Lng: -100}, Moment{Lat: 41, Lng: -100})
	if math.Abs(got-111195) > 500 {
		t.Errorf("distanceM = %v, want ~111195", got)
	}
	if got := distanceM(Moment{Lat: 40, Lng: -100}, Moment{Lat: 40, Lng: -100}); got != 0 {
		t.Errorf("distanceM of a point with itself = %v, want 0", got)
	}
}

// The stopped check is what keeps a parked van from being handed a direction
// invented out of GPS scatter, so it has to sit above the scatter and below a
// real leg.
func TestStoppedMetresSeparatesScatterFromDriving(t *testing.T) {
	// ~5 m apart: parked, jitter only.
	scatter := distanceM(Moment{Lat: 40, Lng: -100}, Moment{Lat: 40.000045, Lng: -100})
	if scatter >= stoppedMetres {
		t.Errorf("GPS scatter of %v m reads as movement", scatter)
	}
	// Ten seconds at 30 mph is ~134 m.
	driving := distanceM(Moment{Lat: 40, Lng: -100}, Moment{Lat: 40.0012, Lng: -100})
	if driving <= stoppedMetres {
		t.Errorf("a driving leg of %v m reads as stopped", driving)
	}
}

// The speed is the baseline distance over the baseline time, and the unit
// helpers are what chat reads — a slip in either is a wrong number on stream.
func TestVelocityUnits(t *testing.T) {
	v := Velocity{SpeedMPS: 10, Moving: true}
	if got := v.KPH(); math.Abs(got-36) > 0.01 {
		t.Errorf("KPH = %v, want 36", got)
	}
	if got := v.MPH(); math.Abs(got-22.37) > 0.01 {
		t.Errorf("MPH = %v, want 22.37", got)
	}
}

// The same 278 m over ten seconds of clip is 28 m/s of driving on a real-time
// clip and a sixth of that on an s2fast one, whose ten clip-seconds stand for
// a minute of road.
func TestVelocityAt_DividesBySpeed(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	conf := 1.0

	for _, tc := range []struct {
		slug    string
		speed   int
		wantMPS float64
	}{
		{"2018_0514_224801_080", 1, 27.8},
		{"20260605144519_000001_s0180", 6, 4.6},
	} {
		vid := insertVideo(t, db, Video{Slug: tc.slug, Speed: tc.speed, CoordConfidence: &conf})
		for ts, lat := range map[int]float64{0: 40.0, 10: 40.0025} {
			if err := db.Exec(`INSERT INTO video_coords (video_id, ts_sec, source_ts_sec, lat, lng, source)
				VALUES (?, ?, ?, ?, -110, 'ocr')`, vid.ID, ts, ts, lat).Error; err != nil {
				t.Fatalf("insert coord: %v", err)
			}
		}
		v, ok := VelocityAt(ctx, vid, 10*time.Second)
		if !ok || !v.Moving {
			t.Fatalf("%s: VelocityAt = %+v, %v; want a moving reading", tc.slug, v, ok)
		}
		if math.Abs(v.SpeedMPS-tc.wantMPS) > 0.2 {
			t.Errorf("%s (speed %d): SpeedMPS = %.1f, want ≈ %.1f", tc.slug, tc.speed, v.SpeedMPS, tc.wantMPS)
		}
	}
}
