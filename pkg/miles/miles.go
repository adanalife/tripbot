package store

import (
	"context"
	"log"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/store"
)

func ForUser(user string) int {
	datastore := context.Background().Value(helpers.StoreKey)
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
