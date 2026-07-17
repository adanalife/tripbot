package helpers

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/bradfitz/latlong"
	"github.com/hako/durafmt"
	"github.com/nathan-osman/go-sunrise"
	"github.com/skratchdot/open-golang/open"
)

// Reverse geocoding (coords -> city/state) moved to pkg/geo, which wraps the
// kelvins/geocoder SDK behind an injectable Geocoder interface. helpers stays
// a pure, dependency-free utility package.

// ProjectRoot returns the root directory of the project
func ProjectRoot() string {
	_, b, _, _ := runtime.Caller(0)
	helperPath := filepath.Dir(b)
	projectRoot := filepath.Join(helperPath, "..", "..")
	absolutePath, _ := filepath.Abs(projectRoot)
	return absolutePath
}

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

// ParseLatLng converts an OCRed string into a LatLng
func ParseLatLng(ocrStr string) (float64, float64, error) {
	// first we have to change the string format
	// from: W111.845329N40.774768
	//   to: 40.774768,111.845329
	nIndex := strings.Index(ocrStr, "N")

	// check if we even found an N
	if nIndex < 0 {
		return 0, 0, errors.New("can't find an N in the string")
	}

	if nIndex == 0 {
		return 0, 0, errors.New("N was the first letter")
	}

	// split up ad lat and long
	lat, _ := strconv.ParseFloat(ocrStr[nIndex+1:], 64)
	lon, _ := strconv.ParseFloat(ocrStr[1:nIndex], 64)

	if lat == 0.0 || lon == 0.0 {
		return lat, lon, errors.New("failed to convert lat or lon to float")
	}

	// western hemisphere assumed: the minus sign is hardcoded rather than
	// parsed (the continental-US bounds check below rejects anything else)
	lon = -lon

	// error on impossible coords
	if lat < -90.0 || lat > 90.0 || lon < -180.0 || lon > 180.0 {
		return lat, lon, errors.New("lat or lon had impossible magnitude")
	}

	// skip anything outside of the continental US (for error correction)
	if lat < 25.7 || lat > 49.23 || lon < -124.44 || lon > -66.57 {
		return lat, lon, errors.New("lat or lon outside USA")
	}

	return lat, lon, nil
}

func RemoveNonLetters(input string) string {
	reg, err := regexp.Compile("[^a-zA-Z]+")
	if err != nil {
		slog.Error("error compiling regex", "err", err)
	}
	return reg.ReplaceAllString(input, "")
}

// FileExists simply returns true if a file exists
func FileExists(path string) bool {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return err == nil
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

func OpenInBrowser(url string) {
	slog.Info("opening url in browser", "url", url)
	err := open.Run(url)
	if err != nil {
		slog.Error("error opening browser", "err", err)
	}
}

// TODO: remove this and all darwin-only support
// RunningOnDarwin returns true if we're on darwin (OS X)
func RunningOnDarwin() bool {
	return runtime.GOOS == "darwin"
}

// RunningOnWindows returns true if we're on windows
func RunningOnWindows() bool {
	return runtime.GOOS == "windows"
}

// RunningOnLinux returns true if we're on linux
func RunningOnLinux() bool {
	return runtime.GOOS == "linux"
}

func StripAtSign(username string) string {
	return strings.TrimPrefix(username, "@")
}
