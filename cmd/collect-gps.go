package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/ocr"
)

// this will hold the filename passed in via the CLI
var videoFile string
var current bool

func init() {
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
		videoFile = ocr.GetCurrentVideo()
	}

	// a file was passed in via the CLI
	if videoFile != "" {
		path := ocr.ScreenshotPath(videoFile)
		lat, lon, err := ocr.CoordsFromImage(path)
		if err != nil {
			log.Fatalf("failed to process image: %v", err)
		}
		url := helpers.GoogleMapsURL(lat, lon)
		fmt.Println(url)

	} else {

		// loop over every file in the screencapDir
		err := filepath.Walk(config.ScreencapDir,
			func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				// skip the directory name itself
				if path == config.ScreencapDir {
					return nil
				}

				// actually process the image
				lat, lon, err := ocr.CoordsFromImage(path)
				if err != nil {
					log.Printf("failed to process image: %v", err)
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
