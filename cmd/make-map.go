package main

import (
	"context"
	"flag"
	"fmt"
	"image/png"
	"log"
	"os"
	"strings"

	// "github.com/kr/pretty"
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
	lat := vidStr[nIndex+1:]
	lon := vidStr[1:nIndex]
	coords := fmt.Sprintf("%s,%s", lat, lon)
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

	// just an example
	coords := "W111.845329N40.774768"
	loc, err := parseLatLng(coords)
	if err != nil {
		log.Fatalf("fatal error: %s", err)
	}

	r := &maps.StaticMapRequest{
		Center:   *center,
		Zoom:     *zoom,
		Size:     "800x600", // *size,
		Scale:    *scale,
		Format:   maps.Format(*format),
		Language: *language,
		Region:   *region,
		MapType:  maps.MapType(*maptype),
		Visible:  []maps.LatLng{loc},
	}

	img, err := client.StaticMap(context.Background(), r)
	if err != nil {
		log.Fatalf("fatal error: %s", err)
	}

	// pretty.Println(resp)
	f, err := os.Create("map.png")
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
}
