package video

import (
	"errors"
	"log"
	"os/exec"
	"path"
	"strings"

	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/ocr"
)

// these are used to keep track of the current video
var CurrentlyPlaying, previouslyPlaying string

// GetCurrentlyPlaying will use lsof to figure out
// which dashcam video is currently playing (seriously)
func GetCurrentlyPlaying() {
	// save the video we used last time
	previouslyPlaying = CurrentlyPlaying
	// figure out whats currently playing
	CurrentlyPlaying = figureOutCurrentVideo()

	if CurrentlyPlaying == previouslyPlaying {
		// it's the same video as last time
	} else {
		// it's a new video, reset the timer
	}
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

func figureOutCurrentVideo() string {
	// run the shell script to get currently-playing video
	scriptPath := path.Join(helpers.ProjectRoot(), "bin/current-file.sh")
	// cmd := fmt.Sprintf("/usr/bin/cd %s && %s", helpers.ProjectRoot(), scriptPath)
	out, err := exec.Command(scriptPath).Output()
	if err != nil {
		log.Printf("failed to run script: %v", err)
		return ""
	}
	return strings.TrimSpace(string(out))
}
