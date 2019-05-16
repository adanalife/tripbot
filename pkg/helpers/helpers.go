package helpers

import (
	"path/filepath"
	"runtime"
	"time"
)

func ProjectRoot() string {
	_, b, _, _ := runtime.Caller(0)
	basepath := filepath.Dir(b)
	return basepath
}

func DurationToMiles(dur time.Duration) int {
	return int(dur.Minutes() / 10)
}

// returns true if a given user should be ignored
func UserIsIgnored(user string) bool {
	for _, ignored := range IgnoredUsers {
		if user == ignored {
			return true
		}
	}
	return false
}
