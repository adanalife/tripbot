package chatbot

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/adanalife/tripbot/pkg/feature"
)

// totalMilesOdds is the chance a given rotation tick shows the lifetime
// "Total Miles" board. It stays rare because it barely moves: the same names
// sit at the top of it for months.
const totalMilesOdds = 0.05

// boardShare is what remains, split six ways. The two long-running boards take
// two shares each and the two guessing-game boards take one, so a guessr board
// comes up about half as often as a miles board — new, but not at the expense
// of the boards viewers have been climbing all month. The two together add up
// to one miles board's worth of screen time.
const boardShare = (1 - totalMilesOdds) / 6

// guessrBoardFlagKey gates the two guessing-game boards. Off, the rotation is
// exactly the three boards it was before them — not a fetch that renders
// nothing, which would spend a rotation slot on a fallback. The overlay is
// live on a broadcast and the game is a service outside this cluster, so the
// useful property is being able to take the boards off screen from the console
// without a deploy.
const guessrBoardFlagKey = "chatbot.guessr_leaderboard"

type leaderboardKind int

const (
	guessLeaderboard leaderboardKind = iota
	monthlyMilesLeaderboard
	totalMilesLeaderboard
	guessrDailyLeaderboard
	guessrMonthlyLeaderboard
)

// pickLeaderboard maps a [0,1) roll onto a leaderboard choice. With guessr off
// the split is the three-board one that predates the guessing game, so the flag
// reverts the rotation rather than leaving a hole in it.
func pickLeaderboard(roll float64, guessr bool) leaderboardKind {
	if !guessr {
		switch {
		case roll < totalMilesOdds:
			return totalMilesLeaderboard
		case roll < totalMilesOdds+(1-totalMilesOdds)/2:
			return guessLeaderboard
		default:
			return monthlyMilesLeaderboard
		}
	}
	switch {
	case roll < totalMilesOdds:
		return totalMilesLeaderboard
	case roll < totalMilesOdds+2*boardShare:
		return guessLeaderboard
	case roll < totalMilesOdds+4*boardShare:
		return monthlyMilesLeaderboard
	case roll < totalMilesOdds+5*boardShare:
		return guessrDailyLeaderboard
	default:
		return guessrMonthlyLeaderboard
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
	// A system-level eval: the rotation is a background tick with no user
	// behind it, so the flag is on or off for the whole channel.
	guessr := a.Flags.Bool(ctx, guessrBoardFlagKey, feature.EvalContext{
		Channel: a.Cfg.ChannelName,
		Env:     a.Cfg.Environment,
	})
	title, rows := a.fetchLeaderboard(ctx, pickLeaderboard(roll, guessr))
	if len(rows) == 0 {
		title, rows = a.fetchLeaderboard(ctx, monthlyMilesLeaderboard)
	}
	if len(rows) == 0 {
		return
	}
	a.Onscreens.ShowLeaderboard(ctx, title, rows)
}

// fetchLeaderboard returns the overlay title and rows for the given kind.
// Titles match the ones the corresponding chat commands use. Everything it
// returns goes straight to the overlay, so each board is fetched at its
// onscreen size rather than trimmed afterwards.
func (a *App) fetchLeaderboard(ctx context.Context, kind leaderboardKind) (string, [][]string) {
	switch kind {
	case totalMilesLeaderboard:
		return "Total Miles", overlayRows(a.Sessions.LifetimeLeaderboard(), onscreenRows)
	case guessLeaderboard:
		return "Correct Guesses This Month", a.Scoreboards.TopGuesses(ctx, onscreenRows)
	case guessrDailyLeaderboard:
		title, rows := a.guessrLeaderboard(ctx, "daily", "2006-01-02", "January 2")
		return title, overlayRows(rows, onscreenRows)
	case guessrMonthlyLeaderboard:
		title, rows := a.guessrLeaderboard(ctx, "monthly", "2006-01", "January")
		return title, overlayRows(rows, guessrMonthlyRows)
	default:
		return a.Scoreboards.MilesMonth() + " Miles", a.Scoreboards.TopMiles(ctx, onscreenRows)
	}
}

// guessrLeaderboard fetches a board from the guessing game and titles it after
// the span it covers, e.g. "August 1 Guessr" — matching the "July Miles" shape
// the miles boards use.
//
// A failure is a log line and no rows, which the caller reads as an empty board
// and falls back from. The game lives outside this cluster, so it being
// unreachable is an ordinary condition rather than an error worth a slot: a
// missed board costs one rotation tick.
func (a *App) guessrLeaderboard(ctx context.Context, board, parse, display string) (string, [][]string) {
	period, rows, err := guessrBoard(ctx, board)
	if err != nil {
		slog.ErrorContext(ctx, "could not fetch guessr leaderboard", "err", err, "board", board)
		return "", nil
	}
	title := "Guessr"
	if when, err := time.Parse(parse, period); err == nil {
		title = when.Format(display) + " Guessr"
	}
	return title, rows
}
