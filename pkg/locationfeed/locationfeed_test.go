package locationfeed

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/video"
)

// fakeCity is a CityLookup returning a fixed result and counting calls so tests
// can assert the geocode throttle.
type fakeCity struct {
	result string
	err    error
	calls  int
}

func (f *fakeCity) City(_, _ float64) (string, error) {
	f.calls++
	return f.result, f.err
}

// recordingPublisher captures the location/date pairs Emit publishes.
type recordingPublisher struct {
	locations []string
	dates     []string
}

func (r *recordingPublisher) UpdateLocation(_ context.Context, location, date string) error {
	r.locations = append(r.locations, location)
	r.dates = append(r.dates, date)
	return nil
}

// moabClip is a clip with a real GPS fix; DateFilmed drives the formatted date.
func moabClip() video.Video {
	return video.Video{
		State:      "Utah",
		Lat:        38.5733,
		Lng:        -109.5498,
		DateFilmed: time.Date(2018, time.June, 14, 19, 30, 0, 0, time.UTC),
	}
}

func TestEmitPublishesLocationAndDate(t *testing.T) {
	pub := &recordingPublisher{}
	e := New(pub, &fakeCity{result: "Moab, Utah"})

	vid := moabClip()
	e.Emit(context.Background(), vid)

	if len(pub.locations) != 1 {
		t.Fatalf("expected one publish, got %d", len(pub.locations))
	}
	if pub.locations[0] != "Moab, Utah" {
		t.Errorf("location = %q, want %q", pub.locations[0], "Moab, Utah")
	}
	lat, lng, _ := vid.Location()
	wantDate := helpers.ActualDate(vid.DateFilmed, lat, lng).Format("Monday January 2, 2006")
	if pub.dates[0] != wantDate {
		t.Errorf("date = %q, want %q", pub.dates[0], wantDate)
	}
}

func TestThrottlesCityWithinState(t *testing.T) {
	pub := &recordingPublisher{}
	geo := &fakeCity{result: "Moab, Utah"}
	e := New(pub, geo)

	for i := 0; i < 3; i++ {
		e.Emit(context.Background(), moabClip())
	}
	if geo.calls != 1 {
		t.Errorf("expected 1 geocode (throttled), got %d", geo.calls)
	}
	if len(pub.locations) != 3 {
		t.Errorf("expected 3 publishes, got %d", len(pub.locations))
	}
}

func TestStateChangeForcesGeocode(t *testing.T) {
	pub := &recordingPublisher{}
	geo := &fakeCity{result: "Moab, Utah"}
	e := New(pub, geo)

	e.Emit(context.Background(), moabClip())
	geo.result = "Grand Junction, Colorado"
	co := moabClip()
	co.State = "Colorado"
	e.Emit(context.Background(), co)

	if geo.calls != 2 {
		t.Errorf("expected 2 geocodes across a state change, got %d", geo.calls)
	}
	if got := pub.locations[len(pub.locations)-1]; got != "Grand Junction, Colorado" {
		t.Errorf("after state change location = %q, want the Colorado city", got)
	}
}

func TestFlaggedClipSkips(t *testing.T) {
	pub := &recordingPublisher{}
	e := New(pub, &fakeCity{result: "Moab, Utah"})

	flagged := moabClip()
	flagged.Flagged = true
	e.Emit(context.Background(), flagged)

	if len(pub.locations) != 0 {
		t.Errorf("flagged clip should not publish (onscreens holds last): got %d", len(pub.locations))
	}
}

func TestFallsBackToStateWhenGeocodeFails(t *testing.T) {
	pub := &recordingPublisher{}
	e := New(pub, &fakeCity{err: errors.New("maps disabled")})

	e.Emit(context.Background(), moabClip())

	if len(pub.locations) != 1 {
		t.Fatalf("expected one publish, got %d", len(pub.locations))
	}
	if pub.locations[0] != "Utah" {
		t.Errorf("location = %q, want state fallback %q", pub.locations[0], "Utah")
	}
}
