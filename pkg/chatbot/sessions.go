package chatbot

import (
	"context"

	"github.com/adanalife/tripbot/pkg/users"
)

// Sessions is the subset of the pkg/users surface that chatbot commands
// depend on at command-time (user lookups, lifetime-leaderboard reads,
// graceful shutdown of in-memory session state). Tests inject a fake;
// production uses the package-backed realSessions adapter wired in
// defaultApp. Mirrors the Onscreens/VLC/Video/IRC injection pattern.
//
// The IRC-side session lifecycle (LoginIfNecessary / LogoutIfNecessary)
// is intentionally NOT on this interface — those are called from the
// free-function handlers in handlers.go (PrivateMessage / UserJoin /
// UserPart), which aren't App methods yet. They'll move onto App (and
// onto this interface) in a follow-up.
type Sessions interface {
	// Find looks up a user by username. Returns User{ID: 0} for an
	// unknown user (mirrors pkg/users.Find's existing contract).
	Find(ctx context.Context, username string) users.User
	// LifetimeLeaderboard returns the current snapshot of the
	// lifetime-miles leaderboard, a slice of [username, miles] pairs
	// hydrated at startup by users.InitLeaderboard. Read-only from the
	// chatbot's perspective.
	LifetimeLeaderboard() [][]string
	// Shutdown logs out every in-memory session, flushing each user's
	// state to the DB. Called by !shutdown before the process exits.
	Shutdown(ctx context.Context)
	// SetBot flips users.is_bot for a username. Returns
	// gorm.ErrRecordNotFound if the user doesn't exist.
	SetBot(ctx context.Context, username string, isBot bool) error
}

// realSessions delegates to pkg/users.
type realSessions struct{}

func (realSessions) Find(ctx context.Context, username string) users.User {
	return users.Find(ctx, username)
}

func (realSessions) LifetimeLeaderboard() [][]string {
	return users.LifetimeMilesLeaderboard()
}

func (realSessions) Shutdown(ctx context.Context) {
	users.Shutdown(ctx)
}

func (realSessions) SetBot(ctx context.Context, username string, isBot bool) error {
	return users.SetBot(ctx, username, isBot)
}
