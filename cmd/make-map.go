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
	"github.com/dmerrick/danalol-stream/pkg/ocr"
	"googlemaps.github.io/maps"
)

var skipToDate = true
var skipDate = time.Date(2018, time.Month(9), 29, 0, 0, 0, 0, time.UTC)

var googleMapsStyle = []string{
	"element:geometry|color:0x242f3e",
	"element:labels.text.stroke|lightness:-80",
	"feature:administrative|element:labels.text.fill|color:0x746855",
	"feature:administrative.locality|element:labels.text.fill|color:0xd59563",
	"feature:poi|element:labels.text.fill|color:0xd59563",
	"feature:poi.park|element:geometry|color:0x263c3f",
	"feature:poi.park|element:labels.text.fill|color:0x6b9a76",
	"feature:road|element:geometry.fill|color:0x2b3544",
	"feature:road|element:labels.text.fill|color:0x9ca5b3",
	"feature:road.arterial|element:geometry.fill|color:0x38414e",
	"feature:road.arterial|element:geometry.stroke|color:0x212a37",
	"feature:road.arterial|element:labels|visibility:off",
	"feature:road.highway|element:geometry.fill|color:0x746855",
	"feature:road.highway|element:geometry.stroke|color:0x1f2835",
	"feature:road.highway|element:labels|visibility:off",
	"feature:road.highway|element:labels.text.fill|color:0xf3d19c",
	"feature:road.local|visibility:off",
	"feature:road.local|element:geometry.fill|color:0x38414e",
	"feature:road.local|element:geometry.stroke|color:0x212a37",
	"feature:transit|element:geometry|color:0x2f3948",
	"feature:transit.station|element:labels.text.fill|color:0xd59563",
	"feature:water|element:geometry|color:0x17263c",
	"feature:water|element:labels.text.fill|color:0x515c6d",
	"feature:water|element:labels.text.stroke|lightness:-20",
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
			if skipToDate {
				vidTime := helpers.VidStrToDate(imgFilename)
				if vidTime.Before(skipDate) {
					fmt.Println(imgFilename, "ignored")
					return nil
				}
			}

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
			Color:    "0xccff00", // highlighter yellow
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

	mapRequest := &maps.StaticMapRequest{
		MapStyles: googleMapsStyle,
		Center:    loc.String(),
		Zoom:      6,
		Size:      "600x400",
		Scale:     -1,
		Format:    maps.Format(""),
		Language:  "",
		Region:    "",
		MapType:   maps.MapType(""),
		// Paths:    []maps.Path{mapPath},
		Paths:   paths,
		Markers: []maps.Marker{marker},
	}

	img, err := c.StaticMap(context.Background(), mapRequest)
	return img, err
}
