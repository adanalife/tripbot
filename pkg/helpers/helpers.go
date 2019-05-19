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

	"github.com/dmerrick/danalol-stream/pkg/config"
	"googlemaps.github.io/maps"
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
func GoogleMapsURL(coordsStr string) string {
	return fmt.Sprintf("https://www.google.com/maps?q=%s", coordsStr)
}

// ParseLatLng converts an OCRed string into a LatLng
func ParseLatLng(ocrStr string) (maps.LatLng, error) {
	// first we have to change the string format
	// from: W111.845329N40.774768
	//   to: 40.774768,111.845329
	nIndex := strings.Index(ocrStr, "N")

	// check if we even found an N
	if nIndex < 0 {
		empty, _ := maps.ParseLatLng("")
		return empty, errors.New("can't find an N in the string")
	}

	// split up ad lat and long
	lat := ocrStr[nIndex+1:]
	lon := ocrStr[1:nIndex]

	// format the string to make Google Maps happy
	//TODO: I hardcoded the minus sign, better to fix that properly
	coords := fmt.Sprintf("%s,-%s", lat, lon)

	// fmt.Println(coords)

	// now we can just pass the string to the library
	loc, err := maps.ParseLatLng(coords)

	return loc, err
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
