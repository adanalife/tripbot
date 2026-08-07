package chatbot

import (
	"context"
	"fmt"

	"github.com/adanalife/tripbot/pkg/users"
)

// noopScoreboards satisfies Scoreboards for tests that don't care about
// leaderboards — both boards read as empty and a credited guess goes nowhere.
type noopScoreboards struct{}

func (noopScoreboards) TopMiles(_ context.Context, _ int) [][]string   { return nil }
func (noopScoreboards) TopGuesses(_ context.Context, _ int) [][]string { return nil }
func (noopScoreboards) MilesMonth() string                             { return "Month" }
func (noopScoreboards) CreditGuess(_ context.Context, _ *users.User)   {}

// recordingScoreboards stages what each board returns and records the credits
// written to it, so a leaderboard test asserts on rows rather than on the SQL
// that would have fetched them.
type recordingScoreboards struct {
	Calls []string
	// Miles / Guesses are the rows the two boards return.
	Miles, Guesses [][]string
	// Month is what MilesMonth reports; a fixed value keeps overlay-title
	// assertions from depending on when the suite runs.
	Month string
	// Credited lists the usernames CreditGuess was called for, in order.
	Credited []string
}

func (r *recordingScoreboards) TopMiles(_ context.Context, size int) [][]string {
	r.Calls = append(r.Calls, fmt.Sprintf("TopMiles(%d)", size))
	return r.Miles
}

func (r *recordingScoreboards) TopGuesses(_ context.Context, size int) [][]string {
	r.Calls = append(r.Calls, fmt.Sprintf("TopGuesses(%d)", size))
	return r.Guesses
}

func (r *recordingScoreboards) MilesMonth() string {
	if r.Month == "" {
		return "Month"
	}
	return r.Month
}

func (r *recordingScoreboards) CreditGuess(_ context.Context, u *users.User) {
	r.Calls = append(r.Calls, fmt.Sprintf("CreditGuess(%q)", u.Username))
	r.Credited = append(r.Credited, u.Username)
}
