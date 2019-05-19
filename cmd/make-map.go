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

	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/ocr"
	"googlemaps.github.io/maps"
)

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

func makeGoogleMap(c *maps.Client, loc maps.LatLng, pathPoints []maps.LatLng) (image.Image, error) {
	// create the path
	mapPath := maps.Path{
		Location: append(pathPoints, loc),
	}

	// add a marker for current location
	//TODO custom icon
	marker := maps.Marker{
		Location: []maps.LatLng{loc},
	}

	mapRequest := &maps.StaticMapRequest{
		Center:   loc.String(),
		Zoom:     6,
		Size:     "600x400",
		Scale:    -1,
		Format:   maps.Format(""),
		Language: "",
		Region:   "",
		MapType:  maps.MapType(""),
		Paths:    []maps.Path{mapPath},
		Markers:  []maps.Marker{marker},
	}

	img, err := c.StaticMap(context.Background(), mapRequest)
	return img, err
}

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
			// skipTo := time.Date(2018, time.October, 15, 0, 0, 0, 0, time.UTC)
			// vidTime := helpers.VidStrToDate(imgFilename)
			// if vidTime.Before(skipTo) {
			// 	fmt.Println(imgFilename, "ignored")
			// 	return nil
			// }

			// extract the coords from the image
			lat, lon, err := ocr.CoordsFromVideoWithRetry(path)
			if err != nil {
				fmt.Println(imgFilename, "coords not found:", err)
				return nil
			}

			// lat, lon, err := helpers.ParseLatLng(coordStr)
			// if err != nil {
			// 	fmt.Println(imgFilename, "failed to convert str to latlng")
			// 	return nil
			// }

			// create location that the maps API can use
			loc, err := maps.ParseLatLng(fmt.Sprintf("%f,%f", lat, lon))
			if err != nil {
				fmt.Println(imgFilename, "invalid coords", err)
				return nil
			}

			// only update the path every 5 frames
			if index%5 == 0 {
				if len(pathPoints) > 24 {
					// remove the oldest element
					pathPoints = pathPoints[1:]
				}

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
