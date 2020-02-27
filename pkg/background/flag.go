package background

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/onscreens"
	"github.com/dmerrick/danalol-stream/pkg/video"
)

var FlagImage *onscreens.Onscreen

var flagDuration = time.Duration(150 * time.Second)
var flagImageFile = path.Join(helpers.ProjectRoot(), "OBS/flag.jpg")

func InitFlagImage() {
	log.Println("Creating flag image onscreen")
	FlagImage = onscreens.NewImage(flagImageFile)
}

func ShowFlag() {
	cur := video.CurrentlyPlaying
	// copy the image to the live location
	err := os.Link(flagSourceFile(cur.State), flagImageFile)
	if err != nil {
		terrors.Log(err, "error creating image")
	}
	FlagImage.Show("", 10*time.Second)
}

// flagSourceFile returns the full path to a flag image file
func flagSourceFile(state string) string {
	spew.Dump(state)
	fileName := fmt.Sprintf("%s.jpg", strings.ToLower(state))
	return path.Join(helpers.ProjectRoot(), "assets", fileName)
}
