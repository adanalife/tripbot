package chatbot

import (
	"context"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/scoreboards"
	"github.com/adanalife/tripbot/pkg/users"
)

// guessScoreboardTotal is the lifetime correct-guess board, alongside the
// monthly one CurrentGuessScoreboard names. Both are credited together — see
// realScoreboards.CreditGuess.
const guessScoreboardTotal = "guess_state_total"

// Scoreboards is the subset of pkg/scoreboards the chatbot needs at command
// time: reading the two leaderboards and crediting a correct state guess.
// Tests inject a fake; production uses the realScoreboards adapter. Mirrors
// the Sessions/Video/Search injection pattern.
//
// Everything here reads or writes Postgres through pkg/scoreboards' package
// helpers, which reach the process-wide handle. Behind this interface, that is
// the adapter's business — a command asks for a leaderboard, not for a query.
type Scoreboards interface {
	// TopMiles returns the current month's miles leaderboard as [username,
	// miles] rows, longest first, at most size of them.
	TopMiles(ctx context.Context, size int) [][]string
	// TopGuesses returns the current month's correct-guess leaderboard in the
	// same shape, with zero-scorers already filtered out and the counts
	// rendered as integers (a guess count is a whole number).
	TopGuesses(ctx context.Context, size int) [][]string
	// MilesMonth is the display name of the month the miles board covers
	// ("July") — the overlay title, not a lookup key.
	MilesMonth() string
	// CreditGuess records one correct state guess for u, on both the lifetime
	// and the current month's board. Errors are logged rather than returned:
	// the guesser has already been told they were right, and failing the
	// command afterwards would be a worse answer than a missing point.
	CreditGuess(ctx context.Context, u *users.User)
}

// realScoreboards is the production Scoreboards adapter. It holds only the
// config the pkg/scoreboards helpers need; the DB handle is theirs.
type realScoreboards struct {
	cfg *c.TripbotConfig
}

func (r realScoreboards) TopMiles(ctx context.Context, size int) [][]string {
	return scoreboards.TopMilesRows(ctx, r.cfg, size)
}

func (r realScoreboards) TopGuesses(ctx context.Context, size int) [][]string {
	return scoreboards.TopGuessRows(ctx, r.cfg, size)
}

func (r realScoreboards) MilesMonth() string {
	return scoreboards.CurrentMilesMonth()
}

// CreditGuess writes both boards. The pairing lives here rather than at the
// call site because "a correct guess is worth a point" is one fact — a command
// that knew there were two boards could credit one and forget the other.
func (r realScoreboards) CreditGuess(ctx context.Context, u *users.User) {
	u.AddToScore(ctx, guessScoreboardTotal, 1.0)
	u.AddToScore(ctx, scoreboards.CurrentGuessScoreboard(), 1.0)
}
