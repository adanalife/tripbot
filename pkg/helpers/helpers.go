package helpers

import (
	"time"
)

func DurationToMiles(dur time.Duration) int {
	return int(dur.Minutes() / 10)
}

// returns true if a given user should be ignored
func userIsIgnored(user string) bool {
	for _, ignored := range IgnoredUsers {
		if user == ignored {
			return true
		}
	}
	return false
}
