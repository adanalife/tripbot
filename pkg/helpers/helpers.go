package helpers

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/bradfitz/latlong"
	"github.com/hako/durafmt"
	"github.com/nathan-osman/go-sunrise"
)

// Reverse geocoding (coords -> city/state) moved to pkg/geo, which wraps the
// kelvins/geocoder SDK behind an injectable Geocoder interface. helpers stays
// a pure, dependency-free utility package.

// DurationToMiles converts Durations to miles
func DurationToMiles(dur time.Duration) float32 {
	// 0.1mi every 3 minutes
	return float32(0.1 * dur.Minutes() / 3.0)
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

// RunningOnLinux returns true if we're on linux
func RunningOnLinux() bool {
	return runtime.GOOS == "linux"
}

func StripAtSign(username string) string {
	return strings.TrimPrefix(username, "@")
}
