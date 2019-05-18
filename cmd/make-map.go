package main

import (
	"context"
	"flag"
	"fmt"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/dmerrick/danalol-stream/pkg/ocr"
	"googlemaps.github.io/maps"
)

const (
	screencapDir = "/Volumes/usbshare1/first frame of every video"
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
	lat := vidStr[nIndex+1:]
	lon := vidStr[1:nIndex]
	//TODO: I hardcoded the minus sign, better to fix that properly
	coords := fmt.Sprintf("%s,-%s", lat, lon)
	fmt.Println(coords)

	// now we can just pass the string to the library
	loc, err := maps.ParseLatLng(coords)
	if err != nil {
		return loc, err
	}
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
		log.Fatalf("fatal error: %s", err)
	}

	// loop over every file in the screencapDir
	err = filepath.Walk(screencapDir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			// skip the directory name itself
			if path == screencapDir {
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
				log.Fatalf("fatal error: %s", err)
			}

			//TODO: make a CustomIcon?
			marker := maps.Marker{
				Location: []maps.LatLng{loc},
			}

			r := &maps.StaticMapRequest{
				Center:   *center,
				Zoom:     6,         // *zoom,
				Size:     "600x400", // *size,
				Scale:    *scale,
				Format:   maps.Format(*format),
				Language: *language,
				Region:   *region,
				MapType:  maps.MapType(*maptype),
				Markers:  []maps.Marker{marker},
				// Visible:  []maps.LatLng{loc},
			}

			img, err := client.StaticMap(context.Background(), r)
			if err != nil {
				log.Fatalf("fatal error: %s", err)
			}

			// save the file
			imgFilename := fmt.Sprintf("%s.png", path)

			fmt.Println(imgFilename)

			f, err := os.Create(imgFilename)
			if err != nil {
				log.Fatal(err)
			}

			if err := png.Encode(f, img); err != nil {
				f.Close()
				log.Fatal(err)
			}

			if err := f.Close(); err != nil {
				log.Fatal(err)
			}
			return err
		})
	// something went wrong walking the directory
	if err != nil {
		log.Println(err)
	}

}
