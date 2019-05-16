package store

import (
	"log"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/store"
)

func ForUser(user string) int {
	datastore := store.CreateOrFindInContext()
	duration := datastore.DurationForUser(user)
	return DurationToMiles(duration)
}

func DurationToMiles(dur time.Duration) int {
	return int(dur.Minutes() / 10)
}
