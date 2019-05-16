package store

import (
	"context"
	"log"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/helpers"
	// "github.com/dmerrick/danalol-stream/pkg/store"
)

func ForUser(user string) int {
	datastore := helpers.CreateOrFindInContext()
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
