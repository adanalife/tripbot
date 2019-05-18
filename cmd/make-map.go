package main

import (
	"context"
	"flag"
	"fmt"
	"image/png"
	"log"
	"os"
	p "path"
	"path/filepath"

	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/ocr"
	"googlemaps.github.io/maps"
)

var (
	center   = flag.String("center", "", "Center the center of the map, equidistant from all edges of the map.")
	zoom     = flag.Int("zoom", -1, "Zoom the zoom level of the map, which determines the magnification level of the map.")
	size     = flag.String("size", "", "Size defines the rectangular dimensions of the map image.")
	scale    = flag.Int("scale", -1, "Scale affects the number of pixels that are returned.")
	format   = flag.String("format", "", "Format defines the format of the resulting image.")
	maptype  = flag.String("maptype", "", "Maptype defines the type of map to construct.")
	language = flag.String("language", "", "Language defines the language to use for display of labels on map tiles.")
	region   = flag.String("region", "", "Region the appropriate borders to display, based on geo-political sensitivities.")
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

	allMarkers := []maps.Marker{}

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

			// check if file already exists
			if helpers.FileExists(imgFilename) {
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

			marker := maps.Marker{
				//TODO: use a CustomIcon?
				// CustomIcon: maps.CustomIcon{
				// 	IconURL: "/Volumes/usbshare1/minibus.png",
				// 	Anchor:  "topleft",
				// 	Scale:   2,
				// },
				Location: []maps.LatLng{loc},
			}

			allMarkers = append(allMarkers, marker)

			r := &maps.StaticMapRequest{
				Center:   *center,
				Zoom:     6,         // *zoom,
				Size:     "600x400", // *size,
				Scale:    *scale,
				Format:   maps.Format(*format),
				Language: *language,
				Region:   *region,
				MapType:  maps.MapType(*maptype),
				Markers:  allMarkers,
				// Markers:  []maps.Marker{marker},
				// Visible:  []maps.LatLng{loc},
			}

			img, err := client.StaticMap(context.Background(), r)
			if err != nil {
				log.Printf("staticmap fatal error: %s", err)
			}

			fullImgFilename := p.Join(config.MapsOutputDir, imgFilename)

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
