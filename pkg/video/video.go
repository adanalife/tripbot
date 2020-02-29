package video

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/dmerrick/danalol-stream/pkg/background"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	vlcClient "github.com/dmerrick/danalol-stream/pkg/vlc-client"
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
			background.GPSImage.ShowFor("", 60*time.Second)
		} else {
			background.GPSImage.Hide()
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

//TODO: this really shouldnt live in the video pkg,
// but there was an import cycle
func ShowFlag() {
	if helpers.FileExists(background.FlagImageFile) {
		log.Printf("removing %s because it already exists", background.FlagImageFile)
		// remove the existing flag file
		err := os.Remove(background.FlagImageFile)
		if err != nil {
			terrors.Log(err, "error removing old flag image")
		}
	}

	cur := CurrentlyPlaying
	// copy the image to the live location
	err := os.Symlink(flagSourceFile(cur.State), background.FlagImageFile)
	if err != nil {
		terrors.Log(err, "error creating new flag image")
	}
	background.FlagImage.Show("", 10*time.Second)
}

// flagSourceFile returns the full path to a flag image file
func flagSourceFile(state string) string {
	spew.Dump(state, strings.ToLower(state))
	fileName := fmt.Sprintf("%s.jpg", strings.ToLower(state))
	return path.Join(helpers.ProjectRoot(), "assets/flags", fileName)
}
