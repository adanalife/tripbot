package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/video"
	"github.com/kelvins/geocoder"
)

// this will hold the filename passed in via the CLI
var videoFile string
var current bool

func init() {
	//TODO: remove this if it's no longer needed
	// err := godotenv.Load()
	// if err != nil {
	// 	log.Fatal("Error loading .env file")
	// }

	geocoder.ApiKey = c.Conf.GoogleMapsAPIKey

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
		// preload the currently-playing vid
		video.GetCurrentlyPlaying()
		videoFile = video.CurrentlyPlaying.String()
	}

	// a file was passed in via the CLI
	if videoFile != "" {
		vid, err := video.LoadOrCreate(videoFile)
		if err != nil {
			log.Println("unable to create video:", err)
		}
		lat, lon, err := vid.Location()
		if err != nil {
			log.Fatalf("failed to process image: %s", err.Error())
		}
		url := helpers.GoogleMapsURL(lat, lon)
		fmt.Println(url)

	} else {

		// loop over every file in the screencapDir
		err := filepath.Walk(c.Conf.VideoDir,
			func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				// skip the directory name itself
				if path == c.Conf.VideoDir {
					return nil
				}

				// actually process the image
				vid, err := video.LoadOrCreate(path)
				if err != nil {
					log.Println("unable to create video:", err)
					return nil
				}
				lat, lon, err := vid.Location()
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
