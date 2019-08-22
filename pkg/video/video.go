package video

import (
	"log"
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
		// calculate the time running
		durationPlayed := CurrentProgress()
		// reset the stopwatch
		timeStarted = time.Now()
		// set up the video for others to use
		CurrentlyPlaying, err = LoadOrCreate(curVid)
		log.Printf("played %s for %s, now playing %s", preVid, durationPlayed, curVid)
		if err != nil {
			log.Println("unable to create Video from %s: %v", curVid, err)
		}
		//TODO: start and reset a timer here
	}
}

// CurrentProgress represents how long the video has been playing
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
