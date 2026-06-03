package chatbot

import (
	"context"
	"fmt"

	"github.com/adanalife/tripbot/pkg/users"
)

// noopSessions satisfies Sessions for tests that don't care about user
// lookups — Find returns the zero-value (unknown) user, the leaderboard
// reads as empty, and Shutdown is a no-op.
type noopSessions struct{}

func (noopSessions) Find(_ context.Context, _ string) users.User                 { return users.User{} }
func (noopSessions) LifetimeLeaderboard() [][]string                             { return nil }
func (noopSessions) Shutdown(_ context.Context)                                  {}
func (noopSessions) SetBot(_ context.Context, _ string, _ bool) error            { return nil }
func (noopSessions) CurrentMiles(_ context.Context, u users.User) float32        { return u.Miles }
func (noopSessions) CurrentMonthlyMiles(_ context.Context, _ users.User) float32 { return 0 }
func (noopSessions) BonusMiles(_ users.User) float32                             { return 0 }

// recordingSessions captures every call made to it so tests can assert
// the chatbot queried the expected user / leaderboard surfaces.
// Tests can stage what Find / LifetimeLeaderboard return via the
// FindResult / Leaderboard fields. All call records are appended in
// order to Calls.
type recordingSessions struct {
	Calls       []string
	FindResult  users.User
	Leaderboard [][]string
	// SetBotErr is the error SetBot will return for every call.
	SetBotErr error
	// Miles / MonthlyMiles / Bonus stage what the miles methods return.
	Miles, MonthlyMiles, Bonus float32
}

func (r *recordingSessions) Find(_ context.Context, username string) users.User {
	r.Calls = append(r.Calls, fmt.Sprintf("Find(%q)", username))
	return r.FindResult
}

func (r *recordingSessions) LifetimeLeaderboard() [][]string {
	r.Calls = append(r.Calls, "LifetimeLeaderboard()")
	return r.Leaderboard
}

func (r *recordingSessions) Shutdown(_ context.Context) {
	r.Calls = append(r.Calls, "Shutdown()")
}

func (r *recordingSessions) SetBot(_ context.Context, username string, isBot bool) error {
	r.Calls = append(r.Calls, fmt.Sprintf("SetBot(%q, %t)", username, isBot))
	return r.SetBotErr
}

func (r *recordingSessions) CurrentMiles(_ context.Context, u users.User) float32 {
	r.Calls = append(r.Calls, fmt.Sprintf("CurrentMiles(%q)", u.Username))
	return r.Miles
}

func (r *recordingSessions) CurrentMonthlyMiles(_ context.Context, u users.User) float32 {
	r.Calls = append(r.Calls, fmt.Sprintf("CurrentMonthlyMiles(%q)", u.Username))
	return r.MonthlyMiles
}

func (r *recordingSessions) BonusMiles(u users.User) float32 {
	r.Calls = append(r.Calls, fmt.Sprintf("BonusMiles(%q)", u.Username))
	return r.Bonus
}
