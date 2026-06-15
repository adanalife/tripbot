package chatbot

import (
	"context"
	"math/rand/v2"

	"github.com/adanalife/tripbot/pkg/scoreboards"
)

// totalMilesOdds is the chance a given rotation tick shows the lifetime
// "Total Miles" board; the remaining probability mass splits evenly
// between the two monthly boards.
const totalMilesOdds = 0.05

type leaderboardKind int

const (
	guessLeaderboard leaderboardKind = iota
	monthlyMilesLeaderboard
	totalMilesLeaderboard
)

// pickLeaderboard maps a [0,1) roll onto a leaderboard choice.
func pickLeaderboard(roll float64) leaderboardKind {
	switch {
	case roll < totalMilesOdds:
		return totalMilesLeaderboard
	case roll < totalMilesOdds+(1-totalMilesOdds)/2:
		return guessLeaderboard
	default:
		return monthlyMilesLeaderboard
	}
}

// ShowRotatingLeaderboard is the periodic onscreen-leaderboard job: each
// tick it puts one of the three leaderboards on screen, chosen at random.
func (a *App) ShowRotatingLeaderboard(ctx context.Context) {
	a.showRotatingLeaderboard(ctx, rand.Float64())
}

// showRotatingLeaderboard takes the roll as a parameter so tests can pin
// the choice. An empty pick (the guess board has no correct guesses early
// in the month) falls back to monthly miles so the slot isn't wasted.
func (a *App) showRotatingLeaderboard(ctx context.Context, roll float64) {
	title, rows := a.fetchLeaderboard(ctx, pickLeaderboard(roll))
	if len(rows) == 0 {
		title, rows = a.fetchLeaderboard(ctx, monthlyMilesLeaderboard)
	}
	if len(rows) == 0 {
		return
	}
	a.Onscreens.ShowLeaderboard(ctx, title, rows)
}

// fetchLeaderboard returns the overlay title and rows for the given kind.
// Titles match the ones the corresponding chat commands use.
func (a *App) fetchLeaderboard(ctx context.Context, kind leaderboardKind) (string, [][]string) {
	switch kind {
	case totalMilesLeaderboard:
		rows := a.Sessions.LifetimeLeaderboard()
		if len(rows) > leaderboardSize {
			rows = rows[:leaderboardSize]
		}
		return "Total Miles", rows
	case guessLeaderboard:
		return "Correct Guesses This Month", scoreboards.TopGuessRows(ctx, leaderboardSize)
	default:
		return scoreboards.CurrentMilesMonth() + " Miles", scoreboards.TopMilesRows(ctx, leaderboardSize)
	}
}
