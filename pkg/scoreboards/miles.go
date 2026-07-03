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

// previousMonth returns a time inside the previous calendar month. Anchored to
// the first of the current month minus a day so month-length differences can't
// skip a month (time.AddDate(0, -1, 0) on the 31st normalizes forward).
func previousMonth() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
}

// PreviousMilesScoreboard names last month's miles scoreboard.
func PreviousMilesScoreboard() string {
	return "miles_" + previousMonth().Format("2006_01")
}

// PreviousGuessScoreboard names last month's guess scoreboard.
func PreviousGuessScoreboard() string {
	return "guess_state_" + previousMonth().Format("2006_01")
}
