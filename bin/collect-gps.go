package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dmerrick/danalol-stream/pkg/screenshot"
)

const (
	screencapDir     = "/Volumes/usbshare1/first frame of every video"
	croppedPath      = "/Volumes/usbshare1/cropped-corners"
	getCurrentScript = "/Users/dmerrick/other_projects/danalol-stream/bin/current-file.sh"
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
		videoFile = screenshot.GetCurrentVideo()
	}

	if videoFile != "" {
		path := screenshot.ScreenshotPath(videoFile)
		err := processImage(path)
		if err != nil {
			log.Fatalf("failed to process image: %v", err)
		}

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
				err = screenshot.ProcessImage(path)
				return err
			})
		// something went wrong walking the directory
		if err != nil {
			log.Println(err)
		}
	}

}
