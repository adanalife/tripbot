package store

import (
	"log"
	"time"

	"github.com/dmerrick/danalol-stream/config"
)

func ForUser(user string) int {
	datastore := context.Background().Value(config.StoreKey)
	duration, err := store.DurationForUser(user)
	if err != nil {
		//TODO: better error here
		log.Println(err)
	}
	return durationToMiles(duration)
}

func DurationToMiles(dur time.Duration) int {
	return int(dur.Minutes() / 10)
}
