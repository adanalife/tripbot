// Package locationfeed publishes what's known about the currently-playing
// dashcam clip — where it is, what day it was filmed, what the weather was, when
// the sun set — to onscreens-server on a timer. The corner rotators resolve their
// $location / $state / $date / $weather / $sunset variables from it, and on a
// stream where no command can reply they show the location and date passively in
// place of the command hints.
//
// It lives in its own package (not cmd/tripbot) so it's unit-testable —
// cmd/tripbot can't host tests because its banner/autoload import calls
// flag.Parse() at init. It depends only on shared packages (video, helpers,
// onscreens-events) and injected interfaces, so it drags no binary-specific
// config in.
package locationfeed

import (
	"context"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/adanalife/tripbot/pkg/helpers"
	oe "github.com/adanalife/tripbot/pkg/onscreens-events"
	"github.com/adanalife/tripbot/pkg/video"
)

// CityLookup is the slice of reverse-geocoding the feed needs: "City, State"
// for a coordinate. Satisfied by the chatbot's Geocoder (pkg/geo); a fake
// stands in for tests.
type CityLookup interface {
	City(lat, lon float64) (string, error)
}

// WeatherLookup is the slice of historical-weather lookup the feed needs.
// Satisfied by pkg/weather's Archive; a fake stands in for tests.
type WeatherLookup interface {
	Historical(ctx context.Context, when time.Time, lat, lng float64) (string, error)
}

// Publisher is the slice of the onscreens client the feed drives.
type Publisher interface {
	UpdateLocation(ctx context.Context, data oe.LocationData) error
}

// lookupThrottle is the minimum spacing between calls to either external
// service. State, date, and sunset are recomputed every tick (all local math);
// the city costs a Google Geocoding call and the conditions cost an Open-Meteo
// call, and every platform's instance runs its own feed over the same footage, so
// the throttle is what keeps four copies of the same question from costing four
// times the API budget.
const lookupThrottle = 5 * time.Minute

// lookupCell is the coordinate grid a lookup is considered current for: 0.05° is
// roughly 3 miles, so a clip that hasn't left its cell doesn't need re-asking.
// It paces the refresh rather than gating what's shown — a cached answer keeps
// being served while the footage drifts out of its cell, because the city three
// miles back is still the city.
const lookupCell = 0.05

// Cache keys for the footage's local calendar day and local hour. Layouts rather
// than time.Truncate, which buckets against the epoch in UTC and so wouldn't
// answer "same day where this was filmed".
const (
	dayKey  = "2006-01-02"
	hourKey = "2006-01-02 15"
)

// Emitter publishes the currently-playing clip's display data. The city and the
// conditions come from external services, so each is cached and re-fetched at
// most once per lookupThrottle; state, date, and sunset are local math and always
// current.
type Emitter struct {
	onscreens Publisher
	geo       CityLookup
	weather   WeatherLookup

	mu sync.Mutex
	// The footage identity the caches belong to: the local calendar day and the
	// state of the clip they were fetched for. A change in either means the
	// footage jumped (a timewarp, or crossing a state line) far enough that the
	// cached answers describe somewhere else entirely.
	day   string
	state string
	// Last successful "City, State" geocode and the cell it was fetched for.
	city     string
	cityCell [2]int
	cityAt   time.Time
	// Last successful conditions description, with the cell and filmed hour it
	// was for — the archive answers per hour per grid cell, so that pair is the
	// whole cache key.
	conditions     string
	conditionsCell [2]int
	conditionsHour string
	conditionsAt   time.Time
}

// New returns an Emitter that publishes to onscreens, geocodes via geo, and
// looks conditions up via weather.
func New(onscreens Publisher, geo CityLookup, weather WeatherLookup) *Emitter {
	return &Emitter{onscreens: onscreens, geo: geo, weather: weather}
}

// Emit publishes the display data for vid. A flagged clip (no GPS fix) is
// skipped — onscreens-server holds the last value (and expires it after its own
// TTL), so a single bad clip doesn't blank the rotator lines.
func (e *Emitter) Emit(ctx context.Context, vid video.Video) {
	lat, lng, err := vid.Location()
	if vid.Flagged || err != nil {
		return
	}
	local := helpers.ActualDate(vid.DateFilmed, lat, lng)

	e.mu.Lock()
	e.invalidateOnJump(local, vid.State)
	place, conditions := e.place(vid.State, lat, lng), e.conditionsFor(ctx, local, lat, lng)
	e.mu.Unlock()

	_ = e.onscreens.UpdateLocation(ctx, oe.LocationData{
		Location: place,
		State:    vid.State,
		Date:     local.Format("Monday January 2, 2006"),
		Weather:  conditions,
		Sunset:   helpers.SunsetAt(vid.DateFilmed, lat, lng).Format("3:04 PM"),
	})
}

// cellOf buckets a coordinate into the lookupCell grid.
func cellOf(lat, lng float64) [2]int {
	return [2]int{int(math.Floor(lat / lookupCell)), int(math.Floor(lng / lookupCell))}
}

// invalidateOnJump drops both cached answers when the footage moved to a
// different day or state, since they now describe somewhere the stream has left.
// Dropping them leaves $location falling back to the state and $weather absent
// (so lines using it don't render) until a fresh lookup lands, which beats
// captioning Utah footage with Wyoming's conditions.
//
// Whether the throttle resets is what separates the two ways that happens. A new
// state on the same day is continuous playback crossing a line — rare, and worth
// asking again immediately so the corner isn't reduced to a state name. A new day
// means the footage jumped (a timewarp lands on an unrelated clip, so almost
// always another day), and viewers can trigger those in bursts — so those wait
// for the throttle rather than turning into a burst of API calls.
//
// Caller holds e.mu.
func (e *Emitter) invalidateOnJump(local time.Time, state string) {
	day := local.Format(dayKey)
	sameDay := day == e.day
	if sameDay && state == e.state {
		return
	}
	e.day, e.state = day, state
	e.city, e.conditions = "", ""
	if sameDay {
		e.cityAt, e.conditionsAt = time.Time{}, time.Time{}
	}
}

// place returns the display location, re-geocoding at most once per
// lookupThrottle once the clip has left the cell the cached city was fetched for.
// Falls back to the clip's state when geocoding is unavailable or hasn't
// succeeded yet.
//
// Caller holds e.mu.
func (e *Emitter) place(state string, lat, lng float64) string {
	cell := cellOf(lat, lng)
	if (e.city == "" || cell != e.cityCell) && time.Since(e.cityAt) > lookupThrottle {
		e.cityAt = time.Now()
		if city, err := e.geo.City(lat, lng); err != nil {
			slog.Warn("locationfeed: city geocode failed", "err", err)
		} else if city != "" {
			e.city, e.cityCell = city, cell
		}
	}
	if e.city != "" {
		return e.city
	}
	return state
}

// conditionsFor returns the weather description for the clip, re-fetching at most
// once per lookupThrottle once the footage has left the cell or the filmed hour
// the cached description was for. Returns "" when no lookup has succeeded yet —
// the rotators treat an empty variable as "this line can't render", which beats
// putting a guess on a 24/7 stream.
//
// Caller holds e.mu.
func (e *Emitter) conditionsFor(ctx context.Context, local time.Time, lat, lng float64) string {
	cell := cellOf(lat, lng)
	hour := local.Format(hourKey)
	stale := e.conditions == "" || cell != e.conditionsCell || hour != e.conditionsHour
	if stale && time.Since(e.conditionsAt) > lookupThrottle {
		e.conditionsAt = time.Now()
		if desc, err := e.weather.Historical(ctx, local, lat, lng); err != nil {
			slog.WarnContext(ctx, "locationfeed: weather lookup failed", "err", err)
		} else if desc != "" {
			e.conditions, e.conditionsCell, e.conditionsHour = desc, cell, hour
		}
	}
	return e.conditions
}
