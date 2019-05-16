package store

import (
	"log"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/store"
)

func ForUser(user string) int {
	datastore := store.CreateOrFindInContext()
	duration, err := datastore.DurationForUser(user)
	if err != nil {
		//TODO: better error here
		log.Println(err)
	}
	return DurationToMiles(duration)
}

func DurationToMiles(dur time.Duration) int {
	return int(dur.Minutes() / 10)
}
