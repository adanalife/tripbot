package main

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	gopath "path"
	"path/filepath"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/dmerrick/danalol-stream/internal/takeout"
	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/video"
	"github.com/joho/godotenv"
	"googlemaps.github.io/maps"
)

var skipToDate = false
var skipDate = time.Date(2018, time.Month(9), 29, 0, 0, 0, 0, time.UTC)

func main() {
	spew.Dump(takeout.LoadLocations())
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// first we must check for required ENV vars
	googleMapsAPIKey := os.Getenv("GOOGLE_MAPS_API_KEY")
	if googleMapsAPIKey == "" {
		panic("You must set GOOGLE_MAPS_API_KEY")
	}

	client, err := maps.NewClient(maps.WithAPIKey(googleMapsAPIKey))
	if err != nil {
		log.Fatalf("client error: %s", err)
	}

	// this will contain the overlay path
	pathPoints := []maps.LatLng{}

	// the number of loops we've gone over
	index := 0
	skipIndex := 0

	// loop over every file in the screencapDir
	err = filepath.Walk(config.VideoDir(),
		func(path string, _ os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			// skip the directory name itself
			if path == config.VideoDir() {
				return nil
			}

			vid, err := video.LoadOrCreate(path)
			if err != nil {
				log.Println("unable to create video:", err)
				return nil
			}

			// this is where we will save the map image
			imgFilename := fmt.Sprintf("%s.png", vid.String())
			fullImgFilename := gopath.Join(config.MapsOutputDir, imgFilename)

			// skip stuff from before this time
			if skipToDate {
				vidTime := vid.Date()
				if vidTime.Before(skipDate) {
					fmt.Println(imgFilename, "ignored")
					return nil
				}
			}

			// extract the coords from the image
			lat, lon, err := vid.Location()
			if err != nil {
				fmt.Println(imgFilename, "coords not found:", err)
				skipIndex = skipIndex + 1
				return nil
			}

			// skip 3/4
			skipIndex = skipIndex + 1
			if skipIndex%4 != 0 {
				// fmt.Println("skipping", imgFilename)
				return nil
			}

			// create location that the maps API can use
			loc, err := maps.ParseLatLng(fmt.Sprintf("%f,%f", lat, lon))
			if err != nil {
				fmt.Println(imgFilename, "invalid coords", err)
				return nil
			}

			// only update the path every 5 frames
			if index%5 == 0 {
				// append the current location to the list
				pathPoints = append(pathPoints, loc)
			}

			// stop here before we make the image
			if helpers.FileExists(fullImgFilename) {
				fmt.Println(imgFilename, "already exists")
				return nil
			}

			// create the google map
			img, err := makeGoogleMap(client, loc, pathPoints)
			if err != nil {
				fmt.Println(imgFilename, "error from gmaps api", err)
				if strings.Contains(err.Error(), "request header list larger than peer") {
					log.Fatalln("gmaps fatal")

				}
				return nil
			}

			// save the image
			err = saveImage(img, fullImgFilename)

			fmt.Println(imgFilename, "created!")
			index = index + 1
			return err
		})

	// something went wrong walking the directory
	if err != nil {
		log.Println(err)
	}

}

// actually create the image
func saveImage(img image.Image, imgPath string) error {
	f, err := os.Create(imgPath)
	if err != nil {
		log.Println(err)
		return err
	}
	defer f.Close()

	err = png.Encode(f, img)
	if err != nil {
		log.Println(err)
	}
	return err
}

// this splits a big array of LatLngs into an array of Paths
func splitPathPoints(pathPoints []maps.LatLng) []maps.Path {
	var divided []maps.Path

	chunkSize := 15
	for i := 0; i < len(pathPoints); i += chunkSize {
		// add 1 so we overlap with next chunk
		end := i + chunkSize + 1

		if end > len(pathPoints) {
			end = len(pathPoints)
		}

		// create a Path using this chunk of points
		mapPath := maps.Path{
			// Color:    "0xccff00", // highlighter yellow
			Location: pathPoints[i:end],
		}

		divided = append(divided, mapPath)
	}
	maxSize := 250
	if len(divided) > maxSize {
		return divided[:maxSize]
	}
	return divided
}

func makeGoogleMap(c *maps.Client, loc maps.LatLng, pathPoints []maps.LatLng) (image.Image, error) {
	// add the current point
	pathPoints = append(pathPoints, loc)
	paths := splitPathPoints(pathPoints)

	// add a marker for current location
	//TODO move off staging
	iconURL := "https://staging.dana.lol/assets/minibus.png"
	marker := maps.Marker{
		Location: []maps.LatLng{loc},
		CustomIcon: maps.CustomIcon{
			IconURL: iconURL,
			Anchor:  "bottom",
			Scale:   4,
		},
	}

	// center the map in the center of the USA
	centerOfUSA := maps.LatLng{Lat: 39.5, Lng: -98.35}

	mapRequest := &maps.StaticMapRequest{
		Center: centerOfUSA.String(),
		Zoom:   4,
		Size:   "800x600",
		// MapStyles: config.GoogleMapsStyle,
		// Center:    loc.String(),
		// Zoom:     5,
		// Size:     "600x400",
		Paths:    paths,
		Scale:    -1,
		Language: "",
		Region:   "",
		Format:   maps.Format(""),
		MapType:  maps.MapType(""),
		Markers:  []maps.Marker{marker},
	}

	img, err := c.StaticMap(context.Background(), mapRequest)
	return img, err
}
