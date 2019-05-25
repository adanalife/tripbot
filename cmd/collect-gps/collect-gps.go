package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/store"
	"github.com/dmerrick/danalol-stream/pkg/video"
	"github.com/joho/godotenv"
)

// this will hold the filename passed in via the CLI
var videoFile string
var current bool

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	if os.Getenv("DASHCAM_DIR") == "" {
		panic("You must set DASHCAM_DIR")
	}
	flag.StringVar(&videoFile, "file", "", "File to load")
	flag.BoolVar(&current, "current", false, "Use currently-playing video")
	flag.Parse()
}

func main() {

	// set videoFile if -current was passed in
	if current {
		// first we check if too many flags were used
		if videoFile != "" {
			log.Fatal("you cannot use -current and -file at the same time")
		}
		videoFile = video.CurrentlyPlaying()
	}

	datastore := store.FindOrCreate(config.DbPath)

	// a file was passed in via the CLI
	if videoFile != "" {
		vid, err := video.New(videoFile)
		if err != nil {
			log.Println("unable to create video: %v", err)
		}
		lat, lon, err := datastore.CoordsFor(vid)
		if err != nil {
			log.Fatalf("failed to process image: %v", err)
		}
		url := helpers.GoogleMapsURL(lat, lon)
		fmt.Println(url)

	} else {

		// loop over every file in the screencapDir
		err := filepath.Walk(config.VideoDir(),
			func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				// skip the directory name itself
				if path == config.VideoDir() {
					return nil
				}

				// actually process the image
				vid, err := video.New(path)
				if err != nil {
					log.Println("unable to create video:", err)
					return nil
				}
				lat, lon, err := datastore.CoordsFor(vid)
				if err != nil {
					log.Printf("failed to process video: %v", err)
					return nil
				}
				url := helpers.GoogleMapsURL(lat, lon)
				fmt.Println(url)
				return err
			})
		// something went wrong walking the directory
		if err != nil {
			log.Println(err)
		}
	}

}
