package scoreboards

import "time"

func CurrentMilesScoreboard() string {
	// uses YYYY_MM format
	return "miles_" + time.Now().Format("2006_01")
}
