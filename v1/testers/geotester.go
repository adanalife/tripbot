package main

import (
	// "fmt"
	// "log"

	"fmt"
	"os"

	"github.com/davecgh/go-spew/spew"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/kelvins/geocoder"
)

func main() {

	// first we must check for required ENV vars
	googleMapsAPIKey, ok := os.LookupEnv("GOOGLE_MAPS_API_KEY")
	if !ok {
		panic("You must set GOOGLE_MAPS_API_KEY")
	}
	geocoder.ApiKey = googleMapsAPIKey

	state, _ := helpers.StateFromCoords(40.775807, -73.97632)
	fmt.Println(state)

	location := geocoder.Location{
		Latitude:  40.775807,
		Longitude: -73.97632,
	}

	addresses, err := geocoder.GeocodingReverse(location)
	if err != nil {
		fmt.Println(err)
	}
	address := addresses[0]

	spew.Dump(addresses)
	spew.Dump(address.State)
}
