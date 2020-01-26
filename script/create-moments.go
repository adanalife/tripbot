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
	"github.com/dmerrick/danalol-stream/pkg/moments"
	"github.com/joho/godotenv"
	"github.com/kelvins/geocoder"
)

// this will hold the filename passed in via the CLI
var screencapFile string
var current bool

func process(slug, offset string) {

	//TODO: make this into a function
	screencapFile := path.Join(slug, offset)

	log.Println("working on:", screencapFile)

	mom, err := moments.LoadOrCreate(screencapFile)
	if err != nil {
		terrors.Log(err, fmt.Sprintf("failed to create moment: %v", screencapFile))
		return
	}

	// lat, lon, err := vid.Location()
	// if err != nil {
	// 	terrors.Log(err, fmt.Sprintf("failed to process image: %v", screencapFile))
	// 	return
	// }
	// url := helpers.GoogleMapsURL(lat, lon)
	// fmt.Println(url)
}

func screencapDir() string {
	// index 11 corresponds to 245 (aka 2m45s)
	// which should have the least screencaps
	return path.Join(config.ScreencapDir(), config.TimestampsToTry[11])
}

func main() {

	log.Println("going to loop over:", screencapDir())

	// make sure we were given enough args
	checkArgs()

	if screencapFile == "" {
		// loop over every file in the screencapDir

		for _, ts := range config.TimestampsToCheck {
		}

		pathToWalk := screencapDir(offset)
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

		// a file was passed in via the CLI
		// so process only that
	} else {
		process(screencapFile)
	}

}

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	googleMapsAPIKey := os.Getenv("GOOGLE_MAPS_API_KEY")
	if googleMapsAPIKey == "" {
		panic("You must set GOOGLE_MAPS_API_KEY")
	}
	geocoder.ApiKey = googleMapsAPIKey

	flag.StringVar(&screencapFile, "file", "", "File to load")
	flag.BoolVar(&current, "current", false, "Use currently-playing video")
	flag.Parse()
}

func checkArgs() {
	// set screencapFile if -current was passed in
	if current {
		// first we check if too many flags were used
		if screencapFile != "" {
			log.Fatal("you cannot use -current and -file at the same time")
		}
		// preload the currently-playing moment
		// moments.GetCurrentlyPlaying()
		screencapFile = moments.CurrentlyPlaying.String()
	}
}
