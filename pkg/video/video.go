package video

import (
	"fmt"
	"log"
	"os/exec"
	"path"
	"strings"
	"time"

	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
	onscreensClient "github.com/adanalife/tripbot/pkg/onscreens-client"
	vlcClient "github.com/adanalife/tripbot/pkg/vlc-client"
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
	if helpers.RunningOnDarwin() {
		curVid = figureOutCurrentVideo()
	} else {
		curVid = vlcClient.CurrentlyPlaying()
	}

	// if the currently-playing video has changed
	if curVid != preVid {
		// reset the stopwatch
		timeStarted = time.Now()

		// share the Video with the system
		CurrentlyPlaying, err = LoadOrCreate(curVid)
		if err != nil {
			terrors.Log(err, fmt.Sprintf("unable to create Video from %s", curVid))
		}

		log.Printf("now playing %s - %s",
			aurora.Yellow(CurrentlyPlaying.File()),
			aurora.Green(helpers.StateToStateAbbrev(CurrentlyPlaying.State)),
		)

		// show the no-GPS image
		if CurrentlyPlaying.Flagged {
			//TODO: kinda cludgy that we hardcode 60s here
			onscreensClient.ShowGPSImage(60 * time.Second)
		} else {
			onscreensClient.HideGPSImage()
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
	outString := strings.TrimSpace(string(out))
	if err != nil {
		terrors.Log(err, "failed to get currently-playing video")
		log.Println(outString)
		return ""
	}
	return outString
}
