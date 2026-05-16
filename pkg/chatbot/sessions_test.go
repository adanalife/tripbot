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

func (noopSessions) Find(_ context.Context, _ string) users.User { return users.User{} }
func (noopSessions) LifetimeLeaderboard() [][]string             { return nil }
func (noopSessions) Shutdown(_ context.Context)                  {}

// recordingSessions captures every call made to it so tests can assert
// the chatbot queried the expected user / leaderboard surfaces.
// Tests can stage what Find / LifetimeLeaderboard return via the
// FindResult / Leaderboard fields. All call records are appended in
// order to Calls.
type recordingSessions struct {
	Calls       []string
	FindResult  users.User
	Leaderboard [][]string
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
