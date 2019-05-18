package helpers

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/config"
	"googlemaps.github.io/maps"
)

// returns the root directory of the project
func ProjectRoot() string {
	_, b, _, _ := runtime.Caller(0)
	helperPath := filepath.Dir(b)
	projectRoot := path.Join(helperPath, "../..")
	return path.Clean(projectRoot)
}

func DurationToMiles(dur time.Duration) int {
	return int(dur.Minutes() / 10)
}

// returns true if a given user should be ignored
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

func ParseLatLng(vidStr string) (maps.LatLng, error) {
	// first we have to change the string format
	// from: W111.845329N40.774768
	//   to: 40.774768,111.845329
	nIndex := strings.Index(vidStr, "N")

	// check if we even found an N
	if nIndex < 0 {
		empty, _ := maps.ParseLatLng("")
		return empty, errors.New("can't find an N in the string")
	}

	// split up ad lat and long
	lat := vidStr[nIndex+1:]
	lon := vidStr[1:nIndex]
	//TODO: I hardcoded the minus sign, better to fix that properly
	coords := fmt.Sprintf("%s,-%s", lat, lon)

	// fmt.Println(coords)

	// now we can just pass the string to the library
	loc, err := maps.ParseLatLng(coords)

	return loc, err
}

// splitOnRegex will is the equivalent of str.split(/regex/)
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

// fileExists simply returns true if a file exists
func FileExists(path string) bool {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return err == nil
}
