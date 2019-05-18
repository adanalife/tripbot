package main

import (
	"context"
	"fmt"
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
	err = filepath.Walk(config.ScreencapDir,
		func(path string, _ os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			// skip the directory name itself
			if path == config.ScreencapDir {
				return nil
			}

			// this is where we will save the map image
			imgFilename := filepath.Base(path)
			fullImgFilename := gopath.Join(config.MapsOutputDir, imgFilename)

			// // check if file already exists
			// if helpers.FileExists(fullImgFilename) {
			// 	fmt.Println(imgFilename, "already exists")
			// 	return nil
			// }

			coordStr, err := ocr.CoordsFromImage(path)
			if err != nil {
				fmt.Println(imgFilename, "coords not found")
				return nil
			}

			loc, err := helpers.ParseLatLng(coordStr)
			if err != nil {
				fmt.Println(imgFilename, "coords invalid")
				return nil
			}

			// check if file already exists
			if helpers.FileExists(fullImgFilename) {
				fmt.Println(imgFilename, "already exists but continuing anyway")
			} else {
				fmt.Println(imgFilename)
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
				return nil
			}

			mapPath := maps.Path{
				Location: append(pathPoints, loc),
			}

			marker := maps.Marker{
				//TODO custom icon
				// CustomIcon: maps.CustomIcon{
				// 	IconURL: "https://emojipedia-us.s3.dualstack.us-west-1.amazonaws.com/thumbs/160/apple/198/minibus_1f690.png",
				// 	Anchor:  "topleft",
				// 	Scale:   2,
				// },
				Location: []maps.LatLng{loc},
			}

			r := &maps.StaticMapRequest{
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
				// Markers:  allMarkers,
				// Visible:  []maps.LatLng{loc},
			}

			img, err := client.StaticMap(context.Background(), r)
			if err != nil {
				fmt.Println(imgFilename, "error from gmaps api", err)
				return nil
			}

			// actually create the image
			f, err := os.Create(fullImgFilename)
			if err != nil {
				log.Println(err)
			}

			if err := png.Encode(f, img); err != nil {
				f.Close()
				log.Println(err)
			}

			if err := f.Close(); err != nil {
				log.Println(err)
			}

			index = index + 1
			return err
		})
	// something went wrong walking the directory
	if err != nil {
		log.Println(err)
	}

}
