package main

import (
	"fmt"
	"log"

	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/store"
	"github.com/dmerrick/danalol-stream/pkg/video"
)

func main() {

	datastore := store.FindOrCreate(config.DbPath)

	videoFile := video.CurrentlyPlaying()

	// a file was passed in via the CLI
	if videoFile != "" {
		vid, err := video.New(videoFile)
		if err != nil {
			log.Println("unable to create Video: %v", err)
		}
		lat, lon, err := datastore.CoordsFor(vid)
		if err != nil {
			log.Fatalf("failed to process image: %v", err)
		}
		url := helpers.GoogleMapsURL(lat, lon)
		fmt.Println(url)
	}
}
