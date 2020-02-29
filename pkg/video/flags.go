package video

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/background"
	"github.com/dmerrick/danalol-stream/pkg/config"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
)

//TODO: this really shouldnt live in the video pkg,
// but there was an import cycle

func ShowFlag() {
	//TODO: this should trigger when a state change event fires instead of every time we run this
	updateFlagFile()
	// actually display the flag
	background.FlagImage.ShowFor("", 10*time.Second)
}

// updateFlagFile replaces the current flag image with the current state flag
func updateFlagFile() {
	if helpers.FileExists(background.FlagImageFile) {
		if config.Verbose {
			log.Printf("removing %s because it already exists", background.FlagImageFile)
		}
		// remove the existing flag file
		err := os.Remove(background.FlagImageFile)
		if err != nil {
			terrors.Log(err, "error removing old flag image")
		}
	}

	// this is the image we should be showing
	newFlagFile := flagSourceFile(CurrentlyPlaying.State)

	// copy the image to the live location
	err := os.Symlink(newFlagFile, background.FlagImageFile)
	if err != nil {
		terrors.Log(err, "error creating new flag image")
	}
}

// flagSourceFile returns the full path to a flag image file
func flagSourceFile(state string) string {
	// convert it to an abbreviation
	abbrev := helpers.StateToStateAbbrev(state)
	// make it lowercase
	abbrev = strings.ToLower(abbrev)
	fileName := fmt.Sprintf("%s.png", abbrev)

	return path.Join(helpers.ProjectRoot(), "assets/flags/medium", fileName)
}
