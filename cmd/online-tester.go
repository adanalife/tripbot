package main

import (
	"fmt"
	"log"
	"os"

	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/store"
	"github.com/dmerrick/danalol-stream/pkg/video"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	if os.Getenv("DASHCAM_DIR") == "" {
		panic("You must set DASHCAM_DIR")
	}

	datastore := store.FindOrCreate("db/tripbot-copy.db")

	videoFile := video.CurrentlyPlaying

	// a file was passed in via the CLI
	if videoFile == "" {
		log.Fatal("no video found")
	}

	vid, err := video.New(videoFile)
	if err != nil {
		log.Println("unable to create Video: %v", err)
	}
	fmt.Println(vid.File())

	lat, lon, err := datastore.CoordsFor(vid)
	if err != nil {
		log.Fatalf("failed to process image: %v", err)
	}
	url := helpers.GoogleMapsURL(lat, lon)
	fmt.Println(url)
}
