// Package locationfeed publishes the currently-playing dashcam clip's location
// and date to onscreens-server on a timer, where the rotators surface it on the
// bot-less YouTube stream in place of the command hints — it's the info the
// !location / !date / !state commands would return, shown passively since no
// command can respond.
//
// It lives in its own package (not cmd/tripbot) so it's unit-testable —
// cmd/tripbot can't host tests because its banner/autoload import calls
// flag.Parse() at init. It depends only on shared packages (video, helpers) and
// injected interfaces, so it drags no binary-specific config in.
package locationfeed

import (
	"context"
	"sync"
	"time"

	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/video"
)

// CityLookup is the slice of reverse-geocoding the feed needs: "City, State"
// for a coordinate. Satisfied by the chatbot's Geocoder (pkg/geo); a fake
// stands in for tests.
type CityLookup interface {
	City(lat, lon float64) (string, error)
}

// Publisher is the slice of the onscreens client the feed drives.
type Publisher interface {
	UpdateLocation(ctx context.Context, location, date string) error
}

// cityRefresh is how often the city is re-geocoded for the SAME state. State and
// date are recomputed every tick (free); the city costs a Google Geocoding API
// call, so it's throttled — consecutive dashcam clips sit close together, so the
// city rarely changes between ticks. A state change forces an immediate
// re-geocode regardless (see place).
const cityRefresh = 5 * time.Minute

// Emitter publishes the currently-playing clip's location + date. The city is
// geocoded at most once per cityRefresh and cached; state + date are always
// current.
type Emitter struct {
	onscreens Publisher
	geo       CityLookup

	mu            sync.Mutex
	lastCity      string // last successful "City, State" geocode for lastState
	lastState     string
	lastGeocodeAt time.Time
}

// New returns an Emitter that publishes to onscreens and geocodes via geo.
func New(onscreens Publisher, geo CityLookup) *Emitter {
	return &Emitter{onscreens: onscreens, geo: geo}
}

// Emit publishes the location + date for vid. A flagged clip (no GPS fix) is
// skipped — onscreens-server holds the last value (and expires it after its own
// TTL), so a single bad clip doesn't blank the rotator line.
func (e *Emitter) Emit(ctx context.Context, vid video.Video) {
	lat, lng, err := vid.Location()
	if vid.Flagged || err != nil {
		return
	}
	date := helpers.ActualDate(vid.DateFilmed, lat, lng).Format("Monday January 2, 2006")
	_ = e.onscreens.UpdateLocation(ctx, e.place(vid, lat, lng), date)
}

// place returns the display location, geocoding the city at most once per
// cityRefresh (or immediately when the state changes) and caching the result
// between calls. Falls back to the clip's state when geocoding is unavailable
// or hasn't succeeded yet.
func (e *Emitter) place(vid video.Video, lat, lng float64) string {
	e.mu.Lock()
	defer e.mu.Unlock()

	if vid.State != e.lastState {
		// New state: drop the old city (it belonged to the previous state) and
		// force a re-geocode this tick rather than showing a city from the wrong
		// state until the throttle elapses.
		e.lastState = vid.State
		e.lastCity = ""
		e.lastGeocodeAt = time.Time{}
	}
	if time.Since(e.lastGeocodeAt) > cityRefresh {
		e.lastGeocodeAt = time.Now()
		if city, err := e.geo.City(lat, lng); err == nil && city != "" {
			e.lastCity = city
		}
	}
	if e.lastCity != "" {
		return e.lastCity
	}
	return vid.State
}
