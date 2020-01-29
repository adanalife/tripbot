package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"

	"github.com/dmerrick/danalol-stream/pkg/config"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/video"
	"github.com/kelvins/geocoder"
)

// this will hold the filename passed in via the CLI
var videoFile string
var current bool

//TODO: this isn't a videoFile anymore
func process(videoFile string) {
	log.Println("working on:", videoFile)

	vid, err := video.LoadOrCreate(videoFile)
	if err != nil {
		terrors.Log(err, fmt.Sprintf("failed to create video: %v", videoFile))
		return
	}

	lat, lon, err := vid.Location()
	if err != nil {
		terrors.Log(err, fmt.Sprintf("failed to process image: %v", videoFile))
		return
	}
	url := helpers.GoogleMapsURL(lat, lon)
	fmt.Println(url)
}

func screencapDir() string {
	// index 11 corresponds to 245 (aka 2m45s)
	// which should have the least screencaps
	return path.Join(config.ScreencapDir, config.TimestampsToTry[11])
}

func main() {

	log.Println("going to loop over:", screencapDir())

	// make sure we were given enough args
	checkArgs()

	// a file was passed in via the CLI
	if videoFile != "" {
		process(videoFile)

	} else {
		// loop over every file in the screencapDir
		pathToWalk := screencapDir()
		err := filepath.Walk(pathToWalk,
			func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				// skip the directory name itself
				if path == pathToWalk {
					return nil
				}

				// actually process the image
				process(path)

				return err
			})
		if err != nil {
			terrors.Log(err, "something went wrong walking the directory")
		}
	}

}

func init() {
	//TODO: remove if unnecessary
	// err := godotenv.Load()
	// if err != nil {
	// 	log.Fatal("Error loading .env file")
	// }
	geocoder.ApiKey = config.GoogleMapsAPIKey

	flag.StringVar(&videoFile, "file", "", "File to load")
	flag.BoolVar(&current, "current", false, "Use currently-playing video")
	flag.Parse()
}

func checkArgs() {
	// set videoFile if -current was passed in
	if current {
		// first we check if too many flags were used
		if videoFile != "" {
			log.Fatal("you cannot use -current and -file at the same time")
		}
		// preload the currently-playing vid
		video.GetCurrentlyPlaying()
		videoFile = video.CurrentlyPlaying.String()
	}
}
