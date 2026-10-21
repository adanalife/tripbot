package chatbot

import (
	"context"
	"fmt"
	"time"

	"github.com/adanalife/tripbot/pkg/scoreboards"
	"github.com/adanalife/tripbot/pkg/users"
)

// noopScoreboards satisfies Scoreboards for tests that don't care about
// leaderboards — both boards read as empty and a credited guess goes nowhere.
type noopScoreboards struct{}

func (noopScoreboards) TopMiles(_ context.Context, _ int) [][]string         { return nil }
func (noopScoreboards) TopGuesses(_ context.Context, _ int) [][]string       { return nil }
func (noopScoreboards) MilesMonth() string                                   { return "Month" }
func (noopScoreboards) LastMonthMiles(_ context.Context, _ int) [][]string   { return nil }
func (noopScoreboards) LastMonthGuesses(_ context.Context, _ int) [][]string { return nil }
func (noopScoreboards) LastMonth() time.Time                                 { return time.Time{} }
func (noopScoreboards) CreditGuess(_ context.Context, _ *users.User)         {}
func (noopScoreboards) Wrapped(_ context.Context, _ string, _ time.Time) (scoreboards.Wrapped, error) {
	return scoreboards.Wrapped{}, nil
}

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
	// LastMiles / LastGuesses are the frozen boards LastMonthMiles and
	// LastMonthGuesses return; Last is what LastMonth reports, fixed for the
	// same reason as Month.
	LastMiles, LastGuesses [][]string
	Last                   time.Time
	// Credited lists the usernames CreditGuess was called for, in order.
	Credited []string
	// WrappedStats / WrappedErr are what Wrapped answers.
	WrappedStats scoreboards.Wrapped
	WrappedErr   error
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

func (r *recordingScoreboards) LastMonthMiles(_ context.Context, size int) [][]string {
	r.Calls = append(r.Calls, fmt.Sprintf("LastMonthMiles(%d)", size))
	return r.LastMiles
}

func (r *recordingScoreboards) LastMonthGuesses(_ context.Context, size int) [][]string {
	r.Calls = append(r.Calls, fmt.Sprintf("LastMonthGuesses(%d)", size))
	return r.LastGuesses
}

func (r *recordingScoreboards) LastMonth() time.Time {
	if r.Last.IsZero() {
		return time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	}
	return r.Last
}

func (r *recordingScoreboards) CreditGuess(_ context.Context, u *users.User) {
	r.Calls = append(r.Calls, fmt.Sprintf("CreditGuess(%q)", u.Username))
	r.Credited = append(r.Credited, u.Username)
}

func (r *recordingScoreboards) Wrapped(_ context.Context, username string, at time.Time) (scoreboards.Wrapped, error) {
	r.Calls = append(r.Calls, fmt.Sprintf("Wrapped(%q, %s)", username, at.UTC().Format("2006-01")))
	return r.WrappedStats, r.WrappedErr
}
