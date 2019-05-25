package main

import (
	// "fmt"
	// "log"

	"log"
	"os"

	owm "github.com/briandowns/openweathermap"
	"github.com/davecgh/go-spew/spew"
	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/store"
	"github.com/dmerrick/danalol-stream/pkg/video"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()
	datastore := store.FindOrCreate(config.DBPath)

	vidStr := "2018_0920_172817_007.MP4"
	vid, _ := video.New(vidStr)
	lat, lon, _ := datastore.CoordsFor(vid)

	realDate := helpers.ActualDate(vid.Date(), lat, lon)

	w, err := owm.NewHistorical("F", os.Getenv("OWM_API_KEY"))
	if err != nil {
		log.Println("error from openweathermap:", err)
	}
	weather := w.HistoryByCoord(&owm.Coordinates{
		Longitude: lat,
		Latitude:  lon,
	}, &owm.HistoricalParameters{
		Start: realDate.Unix(),
		Cnt:   1,
	})
	spew.Dump(w)
	spew.Dump(weather)
}
