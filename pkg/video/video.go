package video

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/logrusorgru/aurora"
)

// CurrentlyPlaying is the video that is currently playing
var CurrentlyPlaying Video

// these are used to keep track of the current video
var curVid, preVid string
var timeStarted time.Time

// GetCurrentlyPlaying will use lsof to figure out
// which dashcam video is currently playing (seriously)
//TODO: consider making this return a video struct
func GetCurrentlyPlaying() {
	var err error
	// save the video we used last time
	preVid = curVid
	// figure out whats currently playing
	curVid = figureOutCurrentVideo()

	// if the currently-playing video has changed
	if curVid != preVid {
		log.Printf("now playing %s", aurora.Yellow(curVid))

		// reset the stopwatch
		timeStarted = time.Now()

		// share the Video with the system
		CurrentlyPlaying, err = LoadOrCreate(curVid)
		if err != nil {
			terrors.Log(err, fmt.Sprintf("unable to create Video from %s", curVid))
		}

		// show the no-GPS image
		if CurrentlyPlaying.Flagged {
			showNoGPSImage()
		} else {
			hideNoGPSImage()
		}
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
	out, err := exec.Command(scriptPath).Output()
	spew.Dump(out)
	if err != nil {
		terrors.Log(err, "failed to get currently-playing video")
		log.Println(out)
		return ""
	}
	return strings.TrimSpace(string(out))
}

func showNoGPSImage() {
	noGPSSrc := path.Join(helpers.ProjectRoot(), "OBS/GPS.png")
	noGPSDest := path.Join(helpers.ProjectRoot(), "OBS/GPS-live.png")
	os.Link(noGPSSrc, noGPSDest)
}

func hideNoGPSImage() {
	noGPSDest := path.Join(helpers.ProjectRoot(), "OBS/GPS-live.png")
	os.Remove(noGPSDest)
}
