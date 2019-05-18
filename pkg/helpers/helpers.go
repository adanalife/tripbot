package helpers

import (
	"path"
	"path/filepath"
	"runtime"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/config"
)

// returns the root directory of the project
func ProjectRoot() string {
	_, b, _, _ := runtime.Caller(0)
	helperPath := filepath.Dir(b)
	projectRoot := path.Join(helperPath, "../..")
	return path.Clean(projectRoot)
}

func DurationToMiles(dur time.Duration) int {
	return int(dur.Minutes() / 10)
}

// returns true if a given user should be ignored
func UserIsIgnored(user string) bool {
	for _, ignored := range config.IgnoredUsers {
		if user == ignored {
			return true
		}
	}
	return false
}
