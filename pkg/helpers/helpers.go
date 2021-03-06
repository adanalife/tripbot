package helpers

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/bradfitz/latlong"
	"github.com/davecgh/go-spew/spew"
	"github.com/hako/durafmt"
	"github.com/kelvins/geocoder"
	"github.com/logrusorgru/aurora"
	"github.com/nathan-osman/go-sunrise"
	"github.com/skratchdot/open-golang/open"
)

func CityFromCoords(lat, lon float64) (string, error) {
	location := geocoder.Location{Latitude: lat, Longitude: lon}

	addresses, err := geocoder.GeocodingReverse(location)
	if err != nil {
		return "", err
	}

	address := addresses[0]
	addStr := fmt.Sprintf("%s, %s", address.City, address.State)
	if address.City == "" {
		addStr = fmt.Sprintf("Somewhere in %s", address.State)
	}
	return addStr, err
}

func StateFromCoords(lat, lon float64) (string, error) {
	location := geocoder.Location{Latitude: lat, Longitude: lon}

	addresses, err := geocoder.GeocodingReverse(location)
	if err != nil {
		spew.Dump(err)
		return "", err
	}
	return addresses[0].State, err
}

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
//TODO find query param for zoom level
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

	//TODO: this is lazy
	// I hardcoded the minus sign instead of handling other hemispheres
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

func RemoveNonLetters(input string) string {
	reg, err := regexp.Compile("[^a-zA-Z]+")
	if err != nil {
		terrors.Log(err, "error compiling regex")
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

// InvertMap takes a string map and returns it as value->key
func InvertMap(m map[string]string) map[string]string {
	n := make(map[string]string)
	for k, v := range m {
		n[v] = k
	}
	return n
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

func sunriseSunset(utcDate time.Time, lat, long float64) (time.Time, time.Time) {
	rise, set := sunrise.SunriseSunset(
		lat, long,
		utcDate.Year(), utcDate.Month(), utcDate.Day(),
	)
	return ActualDate(rise, lat, long), ActualDate(set, lat, long)
}

//TODO: text the admin if it errors opening browser?
func OpenInBrowser(url string) {
	log.Println("opening url")
	err := open.Run(url)
	if err != nil {
		log.Println(aurora.Red("error opening browser"), err)
	}
}

//TODO: remove this and all darwin-only support
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

// this nastiness taken from:
// https://gist.github.com/davidnewhall/3627895a9fc8fa0affbd747183abca39
// Write a pid file, but first make sure it doesn't exist with a running pid.
//TODO: consider refactoring to use PidExists()
func WritePidFile(pidFile string) error {
	// Read in the pid file as a slice of bytes.
	if piddata, err := ioutil.ReadFile(pidFile); err == nil {
		// Convert the file contents to an integer.
		if pid, err := strconv.Atoi(string(piddata)); err == nil {
			// Look for the pid in the process list.
			if process, err := os.FindProcess(pid); err == nil {
				// Send the process a signal zero kill.
				if err := process.Signal(syscall.Signal(0)); err == nil {
					// We only get an error if the pid isn't running, or it's not ours.
					return fmt.Errorf("pid already running: %d", pid)
				}
			}
		}
	}
	// If we get here, then the pidfile didn't exist,
	// or the pid in it doesn't belong to the user running this app.
	return ioutil.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0664)
}

func ReadPidFile(pidFile string) int {
	// Read in the pid file as a slice of bytes.
	if piddata, err := ioutil.ReadFile(pidFile); err == nil {
		// Convert the file contents to an integer.
		pid, err := strconv.Atoi(strings.TrimSpace(string(piddata)))
		if err == nil {
			return pid
		}
	}
	// return an invalid pid otherwise
	return -1
}

// https://stackoverflow.com/a/59459658
func PidExists(pid int) (bool, error) {
	if pid <= 0 {
		return false, fmt.Errorf("invalid pid %v", pid)
	}
	proc, err := os.FindProcess(int(pid))
	if err != nil {
		return false, err
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true, nil
	}
	if err.Error() == "os: process already finished" {
		return false, nil
	}
	errno, ok := err.(syscall.Errno)
	if !ok {
		return false, err
	}
	switch errno {
	case syscall.ESRCH:
		return false, nil
	case syscall.EPERM:
		return true, nil
	}
	return false, err
}

// https://stackoverflow.com/a/28672789
func Base64Encode(str string) string {
	return base64.StdEncoding.EncodeToString([]byte(str))
}

func Base64Decode(str string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func StripAtSign(username string) string {
	if username[0] == []byte("@")[0] {
		// strip the @ sign
		username = username[1:]
	}
	return username
}
