package main

import (
	"fmt"
	"log"

	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/video"
)

func main() {
	video.GetCurrentlyPlaying()
	vid := video.CurrentlyPlaying

	fmt.Println(vid.File())

	lat, lon, err := vid.Location()
	if err != nil {
		log.Fatalf("failed to process image: %v", err)
	}
	url := helpers.GoogleMapsURL(lat, lon)
	fmt.Println(url)
}
