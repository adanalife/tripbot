package main

import (
	"fmt"
	"log"
	"os"

	"github.com/dmerrick/danalol-stream/pkg/database"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
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

	// initialize the SQL database
	database.DBCon, err = database.Initialize()
	if err != nil {
		log.Fatal("error initializing the DB", err)
	}

	video.GetCurrentlyPlaying()
	vid := video.CurrentlyPlaying

	if err != nil {
		log.Println("unable to create Video: %v", err)
	}
	fmt.Println(vid.File())

	lat, lon, err := vid.Location()
	if err != nil {
		log.Fatalf("failed to process image: %v", err)
	}
	url := helpers.GoogleMapsURL(lat, lon)
	fmt.Println(url)
}
