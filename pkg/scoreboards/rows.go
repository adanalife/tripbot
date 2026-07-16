package scoreboards

import (
	"context"
	"strings"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
)

// TopMilesRows returns the top-N rows of the current monthly miles
// scoreboard as [username, miles] pairs, ready for leaderboard rendering.
func TopMilesRows(ctx context.Context, cfg *c.TripbotConfig, size int) [][]string {
	return TopUsers(ctx, cfg, CurrentMilesScoreboard(), size)
}

// TopGuessRows returns the top-N rows of the current monthly guess
// scoreboard as [username, guesses] pairs. Zero-scorers are filtered out
// (AddToScoreByName uses FirstOrCreate, so every user who's ever guessed
// has a row — many at 0 early in the month), and the float values are
// rendered as ints. May return an empty slice.
func TopGuessRows(ctx context.Context, cfg *c.TripbotConfig, size int) [][]string {
	var rows [][]string
	for _, pair := range TopUsers(ctx, cfg, CurrentGuessScoreboard(), size) {
		// guesses are ints not floats, so remove the decimal place
		guesses := strings.Split(pair[1], ".")[0]
		if guesses == "0" || guesses == "" {
			continue
		}
		rows = append(rows, []string{pair[0], guesses})
	}
	return rows
}
