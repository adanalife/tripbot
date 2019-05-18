package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"image/png"
	"log"
	"os"
	p "path"
	"path/filepath"
	"strings"

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

func parseLatLng(vidStr string) (maps.LatLng, error) {
	// first we have to change the string format
	// from: W111.845329N40.774768
	//   to: 40.774768,111.845329
	nIndex := strings.Index(vidStr, "N")

	// check if we even found an N
	if nIndex < 0 {
		empty, _ := maps.ParseLatLng("")
		return empty, errors.New("can't find an N in the string")
	}

	// split up ad lat and long
	lat := vidStr[nIndex+1:]
	lon := vidStr[1:nIndex]
	//TODO: I hardcoded the minus sign, better to fix that properly
	coords := fmt.Sprintf("%s,-%s", lat, lon)

	// fmt.Println(coords)

	// now we can just pass the string to the library
	loc, err := maps.ParseLatLng(coords)

	return loc, err
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

	allMarkers := []maps.Marker{}

	// loop over every file in the screencapDir
	err = filepath.Walk(helpers.ScreencapDir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			// skip the directory name itself
			if path == helpers.ScreencapDir {
				return nil
			}

			// this is where we will save the map image
			imgFilename := filepath.Base(path)

			// check if file already exists
			_, err = os.Stat(imgFilename)
			if err == nil {
				fmt.Println(imgFilename, "already exists")
				return nil
			}

			// crop the image
			croppedImage := ocr.CropImage(path)
			// read off the text
			textFromImage := ocr.ReadText(croppedImage)
			// pull out the coords
			coordStr := ocr.ExtractCoords(textFromImage)

			loc, err := parseLatLng(coordStr)
			if err != nil {
				fmt.Println(imgFilename, "parsing error")
				return nil
			}

			fmt.Println(imgFilename)

			//TODO: make a CustomIcon?
			marker := maps.Marker{
				CustomIcon: maps.CustomIcon{
					IconURL: "/Volumes/usbshare1/minibus.png",
					Anchor:  "topleft",
					Scale:   2,
				},
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

			fullImgFilename := p.Join(helpers.MapsOutputDir, imgFilename)

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
