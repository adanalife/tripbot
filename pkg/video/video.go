package video

import (
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/helpers"
)

// CurrentlyPlaying is the video that is currently playing
var CurrentlyPlaying Video

// these are used to keep track of the current video
var curVid, preVid string
var timeStarted time.Time

// GetCurrentlyPlaying will use lsof to figure out
// which dashcam video is currently playing (seriously)
func GetCurrentlyPlaying() {
	var err error
	// save the video we used last time
	preVid = curVid
	// figure out whats currently playing
	curVid = figureOutCurrentVideo()

	// if the currently-playing video has changed
	if curVid != preVid {
		log.Printf("now playing %s", curVid)

		// reset the stopwatch
		timeStarted = time.Now()

		// share the Video with the system
		CurrentlyPlaying, err = LoadOrCreate(curVid)
		if err != nil {
			log.Println("unable to create Video from %s: %v", curVid, err)
		}

		// copy the no-GPS image to a new location
		createNoGPSImageIfNeeded()
	}
}

// CurrentProgress represents how long the video has been playing
// it will be useful eventually for choosing the exact right screenshot
func CurrentProgress() time.Duration {
	return time.Since(timeStarted)
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

// copy the no-GPS image to a new location
func createNoGPSImageIfNeeded() {
	noGPSSrc := path.Join(helpers.ProjectRoot(), "OBS/GPS.png")
	noGPSDest := path.Join(helpers.ProjectRoot(), "OBS/GPS-live.png")
	if CurrentlyPlaying.Flagged {
		log.Println("current vid is flagged, creating image")
		os.Link(noGPSSrc, noGPSDest)
	} else {
		os.Remove(noGPSDest)
	}
}
