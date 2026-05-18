package video

import (
	"context"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/adanalife/tripbot/pkg/helpers"
	onscreensClient "github.com/adanalife/tripbot/pkg/onscreens-client"
	vlcClient "github.com/adanalife/tripbot/pkg/vlc-client"
)

// CurrentlyPlaying is the video that is currently playing
var CurrentlyPlaying Video

// these are used to keep track of the current video
var curVid, preVid string
var timeStarted time.Time

// GetCurrentlyPlaying will use lsof to figure out
// which dashcam video is currently playing (seriously).
// ctx is forward-compat plumbing — vlc-client and onscreens-client don't
// take ctx yet, so it's not propagated into their HTTP calls. Once they do,
// trace spans for cron.video.GetCurrentlyPlaying ticks will nest the
// underlying VLC poll and GPS-image toggles as children.
//TODO: consider making this return a video struct
func GetCurrentlyPlaying(ctx context.Context) {
	var err error

	// save the video we used last time
	preVid = curVid

	// figure out what's currently playing
	if helpers.RunningOnDarwin() {
		curVid = figureOutCurrentVideo(ctx)
	} else {
		curVid = vlcClient.CurrentlyPlaying()
	}

	// if the currently-playing video has changed
	if curVid != preVid {
		// reset the stopwatch
		timeStarted = time.Now()

		// share the Video with the system
		CurrentlyPlaying, err = LoadOrCreate(ctx, curVid)
		if err != nil {
			slog.ErrorContext(ctx, "unable to create Video", "err", err, "file", curVid)
		}

		slog.InfoContext(ctx, "now playing",
			"file", CurrentlyPlaying.File(),
			"state", helpers.StateToStateAbbrev(CurrentlyPlaying.State),
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

func figureOutCurrentVideo(ctx context.Context) string {
	if helpers.RunningOnWindows() {
		slog.ErrorContext(ctx, "can't run script on windows")
		return ""
	}
	// run the shell script to get currently-playing video
	scriptPath := filepath.Join(helpers.ProjectRoot(), "bin", "current-file.sh")
	out, err := exec.Command(scriptPath).Output()
	outString := strings.TrimSpace(string(out))
	if err != nil {
		slog.ErrorContext(ctx, "figureOutCurrentVideo script failed", "err", err, "output", outString)
		return ""
	}
	return outString
}
