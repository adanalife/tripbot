package video

import (
	"errors"
	"fmt"
	"log"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/ocr"
)

var CurrentlyPlaying string

var timestampsToTry = []string{
	"000",
	"015",
	"030",
	"045",
	"100",
	"115",
	"130",
	"145",
	"200",
	"215",
	"230",
	"245",
}

// Videos represent a video file containing dashcam footage
type Video struct {
	//TODO can Slug be private?
	Slug string
}

func New(file string) (Video, error) {
	var newVid Video

	if file == "" {
		return newVid, errors.New("no file provided")
	}
	fileName := path.Base(file)
	dashStr := removeFileExtension(fileName)

	// validate the dash string
	err := validate(dashStr)
	if err != nil {
		return newVid, err
	}
	newVid = Video{dashStr}
	return newVid, err
}

// ex: 2018_0514_224801_013_a_opt
func (v Video) String() string {
	return v.Slug
}

// a DashStr is the string we get from the dashcam
// an example file: 2018_0514_224801_013.MP4
// an example dashstr: 2018_0514_224801_013
// ex: 2018_0514_224801_013
func (v Video) DashStr() string {
	//TODO: this never should have happened, but it did and it crashed the bot
	if len(v.Slug) < 20 {
		return ""
	}
	return v.Slug[:20]
}

// ex: 2018_0514_224801_013.MP4
func (v Video) File() string {
	return fmt.Sprintf("%s.MP4", v.Slug)
}

// ex: /Volumes/.../2018_0514_224801_013.MP4
func (v Video) Path() string {
	return path.Join(config.VideoDir(), v.File())
}

func (v Video) Date() time.Time {
	vidStr := v.String()
	year, _ := strconv.Atoi(vidStr[:4])
	month, _ := strconv.Atoi(vidStr[5:7])
	day, _ := strconv.Atoi(vidStr[7:9])
	hour, _ := strconv.Atoi(vidStr[10:12])
	minute, _ := strconv.Atoi(vidStr[12:14])
	second, _ := strconv.Atoi(vidStr[14:16])

	t := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)
	return t
}

// timestamp is something like 000, 030, 100, etc
func (v Video) screencap(timestamp string) string {
	screencapFile := fmt.Sprintf("%s-%s.png", v.DashStr(), timestamp)
	return path.Join(config.ScreencapDir(), timestamp, screencapFile)
}

func (v Video) CoordsWithRetry() (float64, float64, error) {
	for _, timestamp := range timestampsToTry {
		lat, lon, err := ocr.CoordsFromImage(v.screencap(timestamp))
		if err == nil {
			return lat, lon, err
		}
	}
	return 0, 0, errors.New("none of the screencaps had valid coords")
}

func GetCurrentlyPlaying() {
	// run the shell script to get currently-playing video
	scriptPath := path.Join(helpers.ProjectRoot(), "bin/current-file.sh")
	// cmd := fmt.Sprintf("/usr/bin/cd %s && %s", helpers.ProjectRoot(), scriptPath)
	out, err := exec.Command(scriptPath).Output()
	if err != nil {
		log.Printf("failed to run script: %v", err)
	}
	CurrentlyPlaying = string(out)
	//TODO: remove me
	log.Println("current vid:", CurrentlyPlaying)
}

func validate(dashStr string) error {
	if len(dashStr) < 20 {
		return errors.New("dash string too short")
	}
	shortened := dashStr[:20]

	if strings.HasPrefix(".", shortened) {
		return errors.New("dash string can't be a hidden file")
	}

	//TODO: this should probably live in an init()
	var validDashStr = regexp.MustCompile(`^[_0-9]{20}$`)
	if !validDashStr.MatchString(shortened) {
		return errors.New("dash string did not match regex")
	}
	return nil
}

func removeFileExtension(filename string) string {
	ext := path.Ext(filename)
	return filename[0 : len(filename)-len(ext)]
}
