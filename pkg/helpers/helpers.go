package helpers

import (
	"time"
)

func DurationToMiles(dur time.Duration) int {
	return int(dur.Minutes() / 10)
}
