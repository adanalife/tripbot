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
	index := 0

	// loop over every file in the screencapDir
	err = filepath.Walk(config.ScreencapDir,
		func(path string, info os.FileInfo, err error) error {
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

			// check if file already exists
			if helpers.FileExists(fullImgFilename) {
				fmt.Println(imgFilename, "already exists")
				return nil
			}

			coordStr, err := ocr.CoordsFromImage(path)
			if err != nil {
				fmt.Println(imgFilename, "unable to find coords")
				return nil
			}

			loc, err := helpers.ParseLatLng(coordStr)
			if err != nil {
				fmt.Println(imgFilename, "coords were invalid")
				return nil
			}

			fmt.Println(imgFilename)

			//marker := maps.Marker{
			//	//TODO: use a CustomIcon?
			//	// CustomIcon: maps.CustomIcon{
			//	// 	IconURL: "/Volumes/usbshare1/minibus.png",
			//	// 	Anchor:  "topleft",
			//	// 	Scale:   2,
			//	// },
			//	Location: []maps.LatLng{loc},
			//}

			// update path every 5 loops
			if index%5 == 0 {
				// create a trail
				if len(pathPoints) > 15 {
					// remove the oldest element
					pathPoints = pathPoints[1:]
				}

				// append the current location to the list
				pathPoints = append(pathPoints, loc)
			}
			//TODO move this down?
			index = index + 1

			mapPath := maps.Path{
				Location: pathPoints,
			}

			r := &maps.StaticMapRequest{
				Center:   "",
				Zoom:     6,         // *zoom,
				Size:     "600x400", // *size,
				Scale:    -1,
				Format:   maps.Format(""),
				Language: "",
				Region:   "",
				MapType:  maps.MapType(""),
				Paths:    []maps.Path{mapPath},
				// Markers:  allMarkers,
				// Markers:  []maps.Marker{marker},
				// Visible:  []maps.LatLng{loc},
			}

			img, err := client.StaticMap(context.Background(), r)
			if err != nil {
				log.Printf("staticmap fatal error: %s", err)
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
			return err
		})
	// something went wrong walking the directory
	if err != nil {
		log.Println(err)
	}

}
