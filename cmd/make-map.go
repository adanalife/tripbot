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

	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"googlemaps.github.io/maps"
)

var skipToDate = true
var skipDate = time.Date(2018, time.Month(9), 29, 0, 0, 0, 0, time.UTC)

func main() {
	// first we must check for required ENV vars
	googleMapsAPIKey, ok := os.LookupEnv("GOOGLE_MAPS_API_KEY")
	if !ok {
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

	// loop over every file in the screencapDir
	err = filepath.Walk(config.VideoDir,
		func(path string, _ os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			// skip the directory name itself
			if path == config.VideoDir {
				return nil
			}

			// this is where we will save the map image
			imgFilename := filepath.Base(path)
			imgFilename = helpers.RemoveFileExtension(imgFilename)
			imgFilename = fmt.Sprintf("%s.png", imgFilename)
			fullImgFilename := gopath.Join(config.MapsOutputDir, imgFilename)

			// skip stuff from before this time
			// time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)
			if skipToDate {
				vidTime := helpers.VidStrToDate(imgFilename)
				if vidTime.Before(skipDate) {
					fmt.Println(imgFilename, "ignored")
					return nil
				}
			}

			// extract the coords from the image
			lat, lon, err := datastore.CoordsFromVideoPath(path)
			if err != nil {
				fmt.Println(imgFilename, "coords not found:", err)
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
	iconURL := "https://staging.dana.lol/assets/minibus.png"
	marker := maps.Marker{
		Location: []maps.LatLng{loc},
		CustomIcon: maps.CustomIcon{
			IconURL: iconURL,
			Anchor:  "center",
			Scale:   4,
		},
	}

	// center the map in the center of the USA
	centerOfUSA := maps.LatLng{Lat: 39.5, Lng: -98.35}

	mapRequest := &maps.StaticMapRequest{
		// MapStyles: config.GoogleMapsStyle,
		// Center:    loc.String(),
		Center:   centerOfUSA.String(),
		Paths:    paths,
		Zoom:     4,
		Size:     "800x600",
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
