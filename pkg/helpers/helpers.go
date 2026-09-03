package helpers

import (
	"fmt"
	"math"
	"runtime"
	"strings"
	"time"

	"github.com/bradfitz/latlong"
	"github.com/hako/durafmt"
	"github.com/nathan-osman/go-sunrise"
)

// Reverse geocoding (coords -> city/state) lives in pkg/geo, which wraps the
// kelvins/geocoder SDK behind an injectable Geocoder interface.

// DurationToMiles converts Durations to miles
func DurationToMiles(dur time.Duration) float32 {
	// 0.1mi every 3 minutes
	return float32(0.1 * dur.Minutes() / 3.0)
}

// MilesBetween returns the great-circle (haversine) distance in miles between
// two lat/lng points.
func MilesBetween(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusMiles = 3958.8
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLng := (lng2 - lng1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return earthRadiusMiles * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// GoogleMapsURL returns a google maps link to the coords provided
// TODO find query param for zoom level
func GoogleMapsURL(lat, long float64) string {
	return fmt.Sprintf("https://maps.google.com/?q=%.5f%%2C%.5f&ll=%.5f%%2C%.5f&z=5", lat, long, lat, long)
}

func ActualDate(utcDate time.Time, lat, long float64) time.Time {
	timezone := latlong.LookupZoneName(lat, long)
	location, err := time.LoadLocation(timezone)
	if err != nil {
		panic(err)
	}
	return utcDate.In(location)
}

func SunsetStr(utcDate time.Time, lat, lon float64) string {
	realDate := ActualDate(utcDate, lat, lon)
	_, sunset := sunriseSunset(realDate, lat, lon)
	dateDiff := sunset.Sub(realDate)
	if dateDiff < 0 {
		// it was in the past
		// we dont want to keep the - sign
		dateDiff = -dateDiff
		return fmt.Sprintf("Sunset on this day was %s ago", durafmt.ParseShort(dateDiff))
	}
	return fmt.Sprintf("Sunset on this day is in %s", durafmt.ParseShort(dateDiff))
}

// TimeAgo renders how long ago t was in the coarse units footage timestamps
// call for — "3 years 2 months ago". Under a month it falls back to durafmt's
// finer units ("3 weeks 2 days ago"). Returns "" for a zero or future
// timestamp, so callers can omit the phrase entirely.
func TimeAgo(t time.Time) string {
	return timeAgoSince(t, time.Now())
}

func timeAgoSince(t, now time.Time) string {
	if t.IsZero() || !t.Before(now) {
		return ""
	}
	months := int(now.Month()) - int(t.Month()) + 12*(now.Year()-t.Year())
	if now.Day() < t.Day() {
		// the calendar month hasn't come round yet
		months--
	}
	if months < 1 {
		return durafmt.Parse(now.Sub(t)).LimitFirstN(2).String() + " ago"
	}
	var parts []string
	if years := months / 12; years > 0 {
		parts = append(parts, pluralize(years, "year"))
	}
	if rem := months % 12; rem > 0 {
		parts = append(parts, pluralize(rem, "month"))
	}
	return strings.Join(parts, " ") + " ago"
}

func pluralize(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", unit)
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// SunsetAt returns the moment the sun set on the day utcDate was filmed, in the
// timezone of the filming location — so formatting it yields a clock time a
// viewer of that footage would recognise. Backs the rotators' $sunset variable,
// where a clock time reads better than SunsetStr's relative phrasing.
func SunsetAt(utcDate time.Time, lat, lon float64) time.Time {
	_, sunset := sunriseSunset(ActualDate(utcDate, lat, lon), lat, lon)
	return sunset
}

// IsDaytime reports whether the moment utcDate, filmed at lat/long, fell
// between that day's sunrise and sunset there — i.e. it's daylight footage.
// Backs the !daytime "skip to the next morning" jump.
func IsDaytime(utcDate time.Time, lat, long float64) bool {
	realDate := ActualDate(utcDate, lat, long)
	rise, set := sunriseSunset(realDate, lat, long)
	return realDate.After(rise) && realDate.Before(set)
}

// LocalDate returns utcDate localized to lat/long and truncated to midnight —
// the calendar day the footage belongs to at its filming location. !daytime
// uses it to tell "the following day" from the current clip's day.
func LocalDate(utcDate time.Time, lat, long float64) time.Time {
	d := ActualDate(utcDate, lat, long)
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())
}

func sunriseSunset(utcDate time.Time, lat, long float64) (time.Time, time.Time) {
	rise, set := sunrise.SunriseSunset(
		lat, long,
		utcDate.Year(), utcDate.Month(), utcDate.Day(),
	)
	return ActualDate(rise, lat, long), ActualDate(set, lat, long)
}

// TODO: remove this and all darwin-only support
// RunningOnDarwin returns true if we're on darwin (OS X)
func RunningOnDarwin() bool {
	return runtime.GOOS == "darwin"
}

func StripAtSign(username string) string {
	return strings.TrimPrefix(username, "@")
}
