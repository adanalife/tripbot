package scoreboards

import "time"

func CurrentMilesScoreboard() string {
	// uses YYYY_MM format
	return "miles_" + time.Now().Format("2006_01")
}

// CurrentMilesMonth returns the full name of the month the current monthly
// miles scoreboard covers (e.g. "June"). Uses the same time.Now() basis as
// CurrentMilesScoreboard so the label always matches the board's data.
func CurrentMilesMonth() string {
	return time.Now().Month().String()
}

func CurrentGuessScoreboard() string {
	// uses YYYY_MM format
	return "guess_state_" + time.Now().Format("2006_01")
}
