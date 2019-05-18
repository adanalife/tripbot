package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

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

	// set videoFile if -current was passed n
	if current {
		if videoFile != "" {
			log.Fatal("you cannot use -current and -file at the same time")
		}
		videoFile = ocr.GetCurrentVideo()
	}

	if videoFile != "" {
		path := ocr.ScreenshotPath(videoFile)
		url, err := ocr.ProcessImage(path)
		if err != nil {
			log.Fatalf("failed to process image: %v", err)
		}
		fmt.Println(url)

	} else {

		// loop over every file in the screencapDir
		err := filepath.Walk(screencapDir,
			func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				// skip the directory name itself
				if path == screencapDir {
					return nil
				}

				// actually process the image
				url, err := ocr.ProcessImage(path)
				if err != nil {
					//TODO: error loglevel?
					log.Printf("failed to process image: %v", err)
					return nil
				}
				fmt.Println(url)
				return err
			})
		// something went wrong walking the directory
		if err != nil {
			log.Println(err)
		}
	}

}
