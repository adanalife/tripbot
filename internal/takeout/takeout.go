package takeout

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/helpers"
)

type Locations struct {
	Locations []Location `json:"locations"`
}
type Location struct {
	Timestamp        string  `json:"timestampMs"`
	Latitude         float64 `json:"latitudeE7"`
	Longitude        float64 `json:"longitudeE7"`
	Accuracy         float64 `json:"accuracy"`
	Velocity         float64 `json:"velocity"`
	Altitude         float64 `json:"altitude"`
	VerticalAccuracy float64 `json:"verticalAccuracy"`
}

func LoadLocations() Locations {
	log.Println("loading locations from json...")
	jsonFile, _ := os.Open("loc.json")
	defer jsonFile.Close()

	byteValue, _ := ioutil.ReadAll(jsonFile)
	// var result map[string]interface{}
	var tripLocations, locations Locations

	json.Unmarshal(byteValue, &locations)
	// spew.Dump(locations)
	startDate, _ := time.Parse(time.RFC3339, "2018-02-13T00:00:00Z")
	endDate, _ := time.Parse(time.RFC3339, "2018-05-10T00:00:00Z")

	for i, loc := range locations.Locations {
		fixLatLon(&loc)
		actualDate := helpers.ActualDate(convertTimestamp(loc), loc.Latitude, loc.Longitude)

		if i%3 == 0 && actualDate.After(startDate) && actualDate.Before(endDate) {
			tripLocations.Locations = append(tripLocations.Locations, loc)
			// fmt.Printf("%s %.6f, %.6f\n", actualDate.Format(time.RFC822), loc.Latitude, loc.Longitude)
		}
	}

	log.Println("...done!")
	return tripLocations
}

func convertTimestamp(loc Location) time.Time {
	// parsed, _ := time.Parse(loc.Timestamp)
	parsed, _ := strconv.ParseInt(loc.Timestamp, 10, 64)
	return time.Unix(0, parsed*int64(time.Millisecond))

}

func fixLatLon(loc *Location) {
	// they are stored weird in the google takeout
	loc.Latitude = loc.Latitude / math.Pow10(7)
	loc.Longitude = loc.Longitude / math.Pow10(7)
}
