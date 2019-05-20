package helpers

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/bradfitz/latlong"
	"github.com/dmerrick/danalol-stream/pkg/config"
)

// ProjectRoot returns the root directory of the project
func ProjectRoot() string {
	_, b, _, _ := runtime.Caller(0)
	helperPath := filepath.Dir(b)
	projectRoot := path.Join(helperPath, "../..")
	return path.Clean(projectRoot)
}

// DurationToMiles converts Durations to miles
func DurationToMiles(dur time.Duration) int {
	return int(dur.Minutes() / 10)
}

// UserIsIgnored returns true if a given user should be ignored
func UserIsIgnored(user string) bool {
	for _, ignored := range config.IgnoredUsers {
		if user == ignored {
			return true
		}
	}
	return false
}

// GoogleMapsURL returns a google maps link to the coords provided
//TODO find query param for zoom level
func GoogleMapsURL(lat, long float64) string {
	return fmt.Sprintf("https://maps.google.com/?q=%.5f,%.5f&ll=%.5f,%.5f&z=5", lat, long, lat, long)
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

	// error on impossible coords
	if lat < -90.0 || lat > 90.0 || lon < -180.0 || lon > 180.0 {
		return lat, lon, errors.New("lat or lon had impossible magnitude")
	}

	// skip anything outside of the continental US (for error correction)
	if lat < -25.7 || lat > 49.23 || lon < -124.44 || lon > -66.57 {
		return lat, lon, errors.New("lat or lon outside USA")
	}

	//TODO: I hardcoded the minus sign, better to fix that properly
	return lat, -lon, nil
}

// SplitOnRegex will is the equivalent of str.split(/regex/)
func SplitOnRegex(text string, delimeter string) []string {
	reg := regexp.MustCompile(delimeter)
	indexes := reg.FindAllStringIndex(text, -1)
	laststart := 0
	result := make([]string, len(indexes)+1)
	for i, element := range indexes {
		result[i] = text[laststart:element[0]]
		laststart = element[1]
	}
	result[len(indexes)] = text[laststart:]
	return result
}

// FileExists simply returns true if a file exists
func FileExists(path string) bool {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return err == nil
}

func VidStrToDate(vidStr string) time.Time {
	year, _ := strconv.Atoi(vidStr[:4])
	month, _ := strconv.Atoi(vidStr[5:7])
	day, _ := strconv.Atoi(vidStr[7:9])
	hour, _ := strconv.Atoi(vidStr[10:12])
	minute, _ := strconv.Atoi(vidStr[12:14])
	second, _ := strconv.Atoi(vidStr[14:16])

	t := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)
	return t
}

func ActualDate(utcDate time.Time, lat, long float64) time.Time {
	timezone := latlong.LookupZoneName(lat, long)
	location, err := time.LoadLocation(timezone)
	if err != nil {
		panic(err)
	}
	return utcDate.In(location)
}

func RemoveFileExtension(filename string) string {
	ext := path.Ext(filename)
	return filename[0 : len(filename)-len(ext)]
}
