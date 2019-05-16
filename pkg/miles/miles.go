package miles

import (
	"fmt"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/store"
)

func ForUser(user string) int {
	datastore := store.CreateOrFindInContext()
	fmt.Println(user)
	duration := datastore.DurationForUser(user)
	return DurationToMiles(duration)
}

func DurationToMiles(dur time.Duration) int {
	return int(dur.Minutes() / 10)
}
