package chatbot

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/video"
)

// stubGeocoder answers State with whatever it's staged with, and records the
// coordinate it was asked about so a test can prove which one the command used.
type stubGeocoder struct {
	state     string
	err       error
	city      string
	cityErr   error
	cityCalls int
	gotLat    float64
	gotLng    float64
	numCalls  int
}

func (g *stubGeocoder) City(_, _ float64) (string, error) {
	g.cityCalls++
	return g.city, g.cityErr
}
func (g *stubGeocoder) State(lat, lng float64) (string, error) {
	g.numCalls++
	g.gotLat, g.gotLng = lat, lng
	return g.state, g.err
}

// A clip with a trusted per-moment track answers from the moment on screen, not
// from the clip's single fix — the whole point of the change. The two
// coordinates here are ~1 km apart, about the measured gap between the two.
func TestCurrentSpot_PrefersThePerMomentCoordinate(t *testing.T) {
	vid := newTestVideo("Colorado", 39.5, -105.0, time.Time{})
	app := newTestApp(vid)
	app.Video = &recordingVideo{Vid: vid, Moment: video.Moment{Lat: 39.512, Lng: -105.004}, PlayheadTracked: true}

	s, ok := app.currentSpot(context.Background())
	if !ok {
		t.Fatal("currentSpot reported not-ok for an unflagged clip")
	}
	if s.at.Lat != 39.512 || s.at.Lng != -105.004 {
		t.Errorf("coord = %v,%v, want the playhead's 39.512,-105.004", s.at.Lat, s.at.Lng)
	}
	if !s.atPlayhead {
		t.Error("atPlayhead = false, want true for a tracked clip")
	}
}

// Without a trusted track there's nothing better than the clip's own fix, and
// falling through to a 0,0 would put a bogus pin in chat.
func TestCurrentSpot_FallsBackToTheClipFix(t *testing.T) {
	vid := newTestVideo("Colorado", 39.5, -105.0, time.Time{})
	app := newTestApp(vid)

	s, ok := app.currentSpot(context.Background())
	if !ok {
		t.Fatal("currentSpot reported not-ok for an unflagged clip")
	}
	if s.at.Lat != 39.5 || s.at.Lng != -105.0 {
		t.Errorf("coord = %v,%v, want the clip's 39.5,-105.0", s.at.Lat, s.at.Lng)
	}
	if s.atPlayhead {
		t.Error("atPlayhead = true, want false when there's no trusted track")
	}
}

// !guess and !state grade against the state at the playhead, so a clip that
// crosses a line answers for the half on screen.
func TestState_GeocodesThePerMomentCoordinate(t *testing.T) {
	vid := newTestVideo("Colorado", 39.5, -105.0, time.Time{})
	app := newTestApp(vid)
	geo := &stubGeocoder{state: "Utah"}
	app.Geocoder = geo

	s := spot{vid: vid, at: video.Moment{Lat: 39.512, Lng: -105.004}, atPlayhead: true}
	if got := app.state(context.Background(), s); got != "Utah" {
		t.Errorf("state = %q, want the geocoded Utah", got)
	}
	if geo.gotLat != 39.512 || geo.gotLng != -105.004 {
		t.Errorf("geocoded %v,%v, want the playhead coordinate", geo.gotLat, geo.gotLng)
	}
}

// videos.state is already this coordinate geocoded once at ingest, so asking
// Maps again would spend a call to get the same answer back.
func TestState_ClipLevelCoordinateSkipsTheLookup(t *testing.T) {
	vid := newTestVideo("Colorado", 39.5, -105.0, time.Time{})
	app := newTestApp(vid)
	geo := &stubGeocoder{state: "Utah"}
	app.Geocoder = geo

	s := spot{vid: vid, at: video.Moment{Lat: 39.5, Lng: -105.0}, atPlayhead: false}
	if got := app.state(context.Background(), s); got != "Colorado" {
		t.Errorf("state = %q, want the clip's Colorado", got)
	}
	if geo.numCalls != 0 {
		t.Errorf("geocoder called %d times, want 0 for a clip-level coordinate", geo.numCalls)
	}
}

// A failed lookup must not blank the state: an empty answer makes guessCmd
// refuse every guess, so a Maps outage would take !guess down with it.
func TestState_FailedLookupKeepsTheClipState(t *testing.T) {
	vid := newTestVideo("Colorado", 39.5, -105.0, time.Time{})
	app := newTestApp(vid)

	for name, geo := range map[string]*stubGeocoder{
		"error":       {err: errors.New("maps is down")},
		"no state":    {state: ""},
		"no key sets": {err: errors.New("geo: maps API disabled")},
	} {
		t.Run(name, func(t *testing.T) {
			app.Geocoder = geo
			s := spot{vid: vid, at: video.Moment{Lat: 39.512, Lng: -105.004}, atPlayhead: true}
			if got := app.state(context.Background(), s); got != "Colorado" {
				t.Errorf("state = %q, want the clip's Colorado", got)
			}
		})
	}
}

// A geocoded moment answers from the row, so the common path spends no Maps
// call at all — that saving is the whole reason the columns exist.
func TestPlace_PrefersTheStoredNameOverTheGeocoder(t *testing.T) {
	vid := newTestVideo("Colorado", 39.5, -105.0, time.Time{})
	app := newTestApp(vid)
	geo := &stubGeocoder{}
	app.Geocoder = geo

	s := spot{vid: vid, at: video.Moment{Lat: 39.512, Lng: -105.004, State: "Colorado", City: "Idaho Springs"}, atPlayhead: true}
	if got := app.place(context.Background(), s); got != "Idaho Springs, Colorado" {
		t.Errorf("place = %q, want the stored Idaho Springs, Colorado", got)
	}
	if geo.cityCalls != 0 {
		t.Errorf("geocoder City called %d times, want 0 for a geocoded moment", geo.cityCalls)
	}
}

// Until the geocode pass reaches a row the bot still has to answer, so the live
// geocoder stays as the fallback rather than the command going silent.
func TestPlace_FallsBackToTheGeocoderWhenUngeocoded(t *testing.T) {
	vid := newTestVideo("Colorado", 39.5, -105.0, time.Time{})
	app := newTestApp(vid)
	geo := &stubGeocoder{city: "Golden, Colorado"}
	app.Geocoder = geo

	s := spot{vid: vid, at: video.Moment{Lat: 39.512, Lng: -105.004}, atPlayhead: true}
	if got := app.place(context.Background(), s); got != "Golden, Colorado" {
		t.Errorf("place = %q, want the geocoder's Golden, Colorado", got)
	}
	if geo.cityCalls != 1 {
		t.Errorf("geocoder City called %d times, want 1", geo.cityCalls)
	}
}

// The clip's own name answers a clip whose per-moment track isn't trustworthy
// enough to read — 194 of 4,406 — which is what takes the last live lookup off
// the command path rather than only the common one.
func TestPlace_FallsBackToTheClipsOwnNameBeforeTheGeocoder(t *testing.T) {
	vid := newTestVideo("Wyoming", 43.65, -110.71, time.Time{})
	cityM := 7700.0
	vid.City, vid.CityM = "Kelly", &cityM
	app := newTestApp(vid)
	geo := &stubGeocoder{city: "somewhere from Google"}
	app.Geocoder = geo

	s := spot{vid: vid, at: video.Moment{Lat: 43.65, Lng: -110.71}, atPlayhead: false}
	if got := app.place(context.Background(), s); got != "near Kelly, Wyoming" {
		t.Errorf("place = %q, want the clip's own near Kelly, Wyoming", got)
	}
	if geo.cityCalls != 0 {
		t.Errorf("geocoder City called %d times, want 0 for a named clip", geo.cityCalls)
	}
}

// A clip carries an ingest-time state from the day it lands, so the clip-level
// fallback has to key on the city the pass writes — not on Place() being
// non-empty, which is true for every clip in the corpus already and would take
// the precise answer away before the pass has run anywhere.
func TestPlace_AStateAloneDoesNotPreemptTheGeocoder(t *testing.T) {
	vid := newTestVideo("Colorado", 39.5, -105.0, time.Time{})
	app := newTestApp(vid)
	geo := &stubGeocoder{city: "Golden, Colorado"}
	app.Geocoder = geo

	s := spot{vid: vid, at: video.Moment{Lat: 39.512, Lng: -105.004}, atPlayhead: true}
	if got := app.place(context.Background(), s); got != "Golden, Colorado" {
		t.Errorf("place = %q, want the geocoder's Golden, Colorado", got)
	}
	if geo.cityCalls != 1 {
		t.Errorf("geocoder City called %d times, want 1", geo.cityCalls)
	}
}

// A name without a distance can't be rendered honestly: the same city string
// means "here" at 0 m and "100 km away" at 100 km. A half-filled row degrades to
// the state rather than claiming the nearest town is the current one.
func TestVideoPlace_NeedsBothTheNameAndTheDistance(t *testing.T) {
	vid := newTestVideo("Wyoming", 43.65, -110.71, time.Time{})
	vid.City = "Kelly" // CityM left nil
	if got := vid.Place(); got != "Somewhere in Wyoming" {
		t.Errorf("Place = %q, want the state alone when the distance is missing", got)
	}
}

// Same precedence for state: a stored one wins, and costs nothing.
func TestState_PrefersTheStoredStateOverTheGeocoder(t *testing.T) {
	vid := newTestVideo("Colorado", 39.5, -105.0, time.Time{})
	app := newTestApp(vid)
	geo := &stubGeocoder{state: "Nebraska"}
	app.Geocoder = geo

	s := spot{vid: vid, at: video.Moment{Lat: 39.512, Lng: -105.004, State: "Utah"}, atPlayhead: true}
	if got := app.state(context.Background(), s); got != "Utah" {
		t.Errorf("state = %q, want the stored Utah", got)
	}
	if geo.numCalls != 0 {
		t.Errorf("geocoder State called %d times, want 0 for a geocoded moment", geo.numCalls)
	}
}
