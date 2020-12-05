package onscreensServer

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"time"

	"github.com/adanalife/tripbot/pkg/config"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/video"
)

var FlagImage *Onscreen
var FlagImageFile = path.Join(config.RunDir, "flag.png")

// var flagDuration = time.Duration(150 * time.Second)

func InitFlagImage() {
	log.Println("Creating flag image onscreen")
	FlagImage = NewImage(FlagImageFile)
}

//TODO: this should probably return an error
func ShowFlag(dur time.Duration) {
	//TODO: this should trigger when a state change event fires instead of every time we run this
	updateFlagFile()
	// actually display the flag
	FlagImage.ShowFor("", 10*time.Second)
}

// updateFlagFile replaces the current flag image with the current state flag
func updateFlagFile() {
	if helpers.FileExists(FlagImageFile) {
		if config.Verbose {
			log.Printf("removing %s because it already exists", FlagImageFile)
		}
		// remove the existing flag file
		err := os.Remove(FlagImageFile)
		if err != nil {
			terrors.Log(err, "error removing old flag image")
		}
	}

	vid := video.CurrentlyPlaying
	// find the next unflagged video
	if vid.Flagged {
		vid = vid.Next()
	}

	// this is the image we should be showing
	newFlagFile := flagSourceFile(vid.State)

	// if nothing was returned, we don't have an image to use
	if newFlagFile == "" {
		err := fmt.Errorf("no matching image found")
		terrors.Log(err, "error creating new flag image")
	}

	// copy the image to the live location
	err := os.Symlink(newFlagFile, FlagImageFile)
	if err != nil {
		terrors.Log(err, "error creating new flag image")
	}
}

// flagSourceFile returns the full path to a flag image file
func flagSourceFile(state string) string {
	// convert it to an abbreviation
	abbrev := helpers.StateToStateAbbrev(state)
	// return nothing if nothing was found
	if abbrev == "" {
		return ""
	}
	// make it lowercase
	abbrev = strings.ToLower(abbrev)
	fileName := fmt.Sprintf("%s.png", abbrev)

	return path.Join(helpers.ProjectRoot(), "assets/flags/medium", fileName)
}
