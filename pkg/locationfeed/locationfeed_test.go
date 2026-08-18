package locationfeed

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/helpers"
	oe "github.com/adanalife/tripbot/pkg/onscreens-events"
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

// fakeWeather is a WeatherLookup returning a fixed description and recording the
// times it was asked about, so tests can assert both the throttle and which
// moment was looked up.
type fakeWeather struct {
	result string
	err    error
	asked  []time.Time
}

func (f *fakeWeather) Historical(_ context.Context, when time.Time, _, _ float64) (string, error) {
	f.asked = append(f.asked, when)
	return f.result, f.err
}

// recordingPublisher captures the payloads Emit publishes.
type recordingPublisher struct {
	sent []oe.LocationData
}

func (r *recordingPublisher) UpdateLocation(_ context.Context, data oe.LocationData) error {
	r.sent = append(r.sent, data)
	return nil
}

func (r *recordingPublisher) last(t *testing.T) oe.LocationData {
	t.Helper()
	if len(r.sent) == 0 {
		t.Fatal("nothing published")
	}
	return r.sent[len(r.sent)-1]
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

// newEmitter wires an Emitter over both fakes, staged to succeed.
func newEmitter() (*Emitter, *recordingPublisher, *fakeCity, *fakeWeather) {
	pub := &recordingPublisher{}
	geo := &fakeCity{result: "Moab, Utah"}
	wx := &fakeWeather{result: "Clear sky, 88°F"}
	return New(pub, geo, wx), pub, geo, wx
}

func TestEmitPublishesEveryVariable(t *testing.T) {
	e, pub, _, _ := newEmitter()

	vid := moabClip()
	e.Emit(context.Background(), vid)

	if len(pub.sent) != 1 {
		t.Fatalf("expected one publish, got %d", len(pub.sent))
	}
	got := pub.last(t)
	lat, lng, _ := vid.Location()
	local := helpers.ActualDate(vid.DateFilmed, lat, lng)

	if got.Location != "Moab, Utah" {
		t.Errorf("location = %q, want %q", got.Location, "Moab, Utah")
	}
	if got.State != "Utah" {
		t.Errorf("state = %q, want %q", got.State, "Utah")
	}
	if want := local.Format("Monday January 2, 2006"); got.Date != want {
		t.Errorf("date = %q, want %q", got.Date, want)
	}
	if got.Weather != "Clear sky, 88°F" {
		t.Errorf("weather = %q, want the staged description", got.Weather)
	}
	if want := helpers.SunsetAt(vid.DateFilmed, lat, lng).Format("3:04 PM"); got.Sunset != want {
		t.Errorf("sunset = %q, want %q", got.Sunset, want)
	}
}

// The archive is asked about the clip's *local* time, not the stored UTC — the
// hourly samples come back in local time, so an offset would read the wrong hour.
func TestWeatherIsLookedUpInFootageLocalTime(t *testing.T) {
	e, _, _, wx := newEmitter()

	vid := moabClip()
	e.Emit(context.Background(), vid)

	if len(wx.asked) != 1 {
		t.Fatalf("expected one weather lookup, got %d", len(wx.asked))
	}
	lat, lng, _ := vid.Location()
	if want := helpers.ActualDate(vid.DateFilmed, lat, lng); !wx.asked[0].Equal(want) {
		t.Errorf("looked up %v, want the clip's local time %v", wx.asked[0], want)
	}
}

func TestThrottlesLookupsWithinACell(t *testing.T) {
	e, pub, geo, wx := newEmitter()

	for i := 0; i < 3; i++ {
		e.Emit(context.Background(), moabClip())
	}
	if geo.calls != 1 {
		t.Errorf("expected 1 geocode (throttled), got %d", geo.calls)
	}
	if len(wx.asked) != 1 {
		t.Errorf("expected 1 weather lookup (throttled), got %d", len(wx.asked))
	}
	if len(pub.sent) != 3 {
		t.Errorf("expected 3 publishes, got %d", len(pub.sent))
	}
}

// Drifting out of the cached cell doesn't blank the location: the city a few
// miles back is still the city, so the cached answer is served until the throttle
// allows a fresh look.
func TestCellDriftKeepsServingTheCachedCity(t *testing.T) {
	e, pub, geo, _ := newEmitter()

	e.Emit(context.Background(), moabClip())
	moved := moabClip()
	moved.Lat += 5 * lookupCell // several cells north, same state and day
	e.Emit(context.Background(), moved)

	if geo.calls != 1 {
		t.Errorf("expected the throttle to hold at 1 geocode, got %d", geo.calls)
	}
	if got := pub.last(t).Location; got != "Moab, Utah" {
		t.Errorf("location = %q, want the cached city", got)
	}
}

// Crossing a state line is continuous playback, not a jump, so it re-asks right
// away rather than dropping the corner to a bare state name.
func TestStateChangeForcesFreshLookups(t *testing.T) {
	e, pub, geo, wx := newEmitter()

	e.Emit(context.Background(), moabClip())
	geo.result = "Grand Junction, Colorado"
	co := moabClip()
	co.State = "Colorado"
	e.Emit(context.Background(), co)

	if geo.calls != 2 {
		t.Errorf("expected 2 geocodes across a state change, got %d", geo.calls)
	}
	if len(wx.asked) != 2 {
		t.Errorf("expected 2 weather lookups across a state change, got %d", len(wx.asked))
	}
	if got := pub.last(t).Location; got != "Grand Junction, Colorado" {
		t.Errorf("after state change location = %q, want the Colorado city", got)
	}
}

// A timewarp lands on unrelated footage, so the cached answers are dropped — but
// the throttle holds, because viewers can trigger warps in bursts and each one
// must not cost a pair of API calls. Until a fresh lookup lands, $location falls
// back to the state and $weather is absent rather than wrong.
func TestTimewarpDropsStaleDataWithoutRefetching(t *testing.T) {
	e, pub, geo, wx := newEmitter()

	e.Emit(context.Background(), moabClip())

	warped := moabClip()
	warped.State = "Wyoming"
	warped.Lat, warped.Lng = 43.0, -108.0
	warped.DateFilmed = time.Date(2018, time.March, 7, 15, 0, 0, 0, time.UTC)
	e.Emit(context.Background(), warped)

	if geo.calls != 1 {
		t.Errorf("expected the throttle to hold at 1 geocode across a warp, got %d", geo.calls)
	}
	if len(wx.asked) != 1 {
		t.Errorf("expected the throttle to hold at 1 weather lookup across a warp, got %d", len(wx.asked))
	}
	got := pub.last(t)
	if got.Location != "Wyoming" {
		t.Errorf("location = %q, want the state fallback rather than Utah's city", got.Location)
	}
	if got.Weather != "" {
		t.Errorf("weather = %q, want it absent rather than Utah's conditions", got.Weather)
	}
}

func TestFlaggedClipSkips(t *testing.T) {
	e, pub, _, _ := newEmitter()

	flagged := moabClip()
	flagged.Flagged = true
	e.Emit(context.Background(), flagged)

	if len(pub.sent) != 0 {
		t.Errorf("flagged clip should not publish (onscreens holds last): got %d", len(pub.sent))
	}
}

// A zero DateFilmed reaches Emit two ways — an empty Video from a player that
// answered with nothing, and a freshly-inserted row the import pass hasn't
// stamped — and neither is flagged, so the GPS check alone lets them through.
// Publishing one puts "Monday January 1, 0001" on a rotator that runs
// unprompted, which is worse than holding the previous clip's line.
func TestZeroDateFilmedSkips(t *testing.T) {
	for _, tc := range []struct {
		name string
		vid  video.Video
	}{
		{"empty video from a dead poll", video.Video{}},
		{"real GPS fix, date never stamped", func() video.Video {
			v := moabClip()
			v.DateFilmed = time.Time{}
			return v
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, pub, _, _ := newEmitter()
			e.Emit(context.Background(), tc.vid)
			if len(pub.sent) != 0 {
				t.Errorf("published %+v; a zero DateFilmed must not reach the rotator", pub.sent)
			}
		})
	}
}

func TestFallsBackToStateWhenGeocodeFails(t *testing.T) {
	pub := &recordingPublisher{}
	e := New(pub, &fakeCity{err: errors.New("maps disabled")}, &fakeWeather{result: "Clear sky, 88°F"})

	e.Emit(context.Background(), moabClip())

	if got := pub.last(t).Location; got != "Utah" {
		t.Errorf("location = %q, want state fallback %q", got, "Utah")
	}
}

// A failed weather lookup publishes an absent value, which makes any line using
// $weather ineligible — the rotators would rather skip the line than guess.
func TestEmptyWeatherWhenLookupFails(t *testing.T) {
	pub := &recordingPublisher{}
	e := New(pub, &fakeCity{result: "Moab, Utah"}, &fakeWeather{err: errors.New("archive down")})

	e.Emit(context.Background(), moabClip())

	got := pub.last(t)
	if got.Weather != "" {
		t.Errorf("weather = %q, want empty on lookup failure", got.Weather)
	}
	if got.Location != "Moab, Utah" {
		t.Errorf("a weather failure shouldn't cost the location: %q", got.Location)
	}
}
