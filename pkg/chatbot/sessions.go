package chatbot

import (
	"context"
	"sync"

	"github.com/adanalife/tripbot/pkg/users"
)

// Sessions is the subset of the pkg/users surface that chatbot commands
// depend on at command-time (user lookups, lifetime-leaderboard reads,
// miles computations, graceful shutdown of in-memory session state). Tests
// inject a fake; production uses the realSessions adapter wired in defaultApp.
// Mirrors the Onscreens/VLC/Video/IRC injection pattern.
//
// The IRC-side session lifecycle (LoginIfNecessary / LogoutIfNecessary) and the
// follower/subscriber + login-count reads are intentionally NOT on this
// interface — they're called from the free-function handlers in handlers.go /
// announce.go, which aren't App methods yet, so they reach the installed
// *Sessions through currentSessions() directly. They'll move onto App (and onto
// this interface) when those handlers do.
type Sessions interface {
	// Find looks up a user by username. Returns User{ID: 0} for an
	// unknown user (mirrors pkg/users.Find's existing contract).
	Find(ctx context.Context, username string) users.User
	// LifetimeLeaderboard returns the current snapshot of the
	// lifetime-miles leaderboard, a slice of [username, miles] pairs
	// hydrated at startup by InitLeaderboard. Read-only from the
	// chatbot's perspective.
	LifetimeLeaderboard() [][]string
	// CurrentMiles / CurrentMonthlyMiles / BonusMiles compute a user's miles
	// including the live session bonus, which depends on the session's login
	// map — hence they live on Sessions and take the User.
	CurrentMiles(ctx context.Context, u users.User) float32
	CurrentMonthlyMiles(ctx context.Context, u users.User) float32
	BonusMiles(u users.User) float32
	// Shutdown logs out every in-memory session, flushing each user's
	// state to the DB. Called by !shutdown before the process exits.
	Shutdown(ctx context.Context)
	// SetBot flips users.is_bot for a username. Returns
	// gorm.ErrRecordNotFound if the user doesn't exist.
	SetBot(ctx context.Context, username string, isBot bool) error
}

// sessions is the *users.Sessions realSessions (and the free-function IRC
// handlers) delegate to. cmd/tripbot installs the single process-wide instance
// via SetSessions once it's constructed. nil until then (brief startup window)
// and in tests, which inject their own Sessions fake rather than realSessions —
// so the nil guards below only ever fire pre-install.
var (
	sessionsMu sync.RWMutex
	sessions   *users.Sessions
)

// SetSessions installs the Sessions that realSessions and the IRC handlers
// delegate to. Called from cmd/tripbot once Sessions is constructed.
func SetSessions(s *users.Sessions) {
	sessionsMu.Lock()
	sessions = s
	sessionsMu.Unlock()
}

func currentSessions() *users.Sessions {
	sessionsMu.RLock()
	defer sessionsMu.RUnlock()
	return sessions
}

// realSessions delegates to the installed *users.Sessions, plus pkg/users'
// standalone DB helper (Find, which is not session state).
type realSessions struct{}

func (realSessions) Find(ctx context.Context, username string) users.User {
	return users.Find(ctx, username)
}

func (realSessions) LifetimeLeaderboard() [][]string {
	s := currentSessions()
	if s == nil {
		return nil
	}
	return s.LifetimeLeaderboard()
}

func (realSessions) CurrentMiles(ctx context.Context, u users.User) float32 {
	s := currentSessions()
	if s == nil {
		return u.Miles
	}
	return s.CurrentMiles(ctx, u)
}

func (realSessions) CurrentMonthlyMiles(ctx context.Context, u users.User) float32 {
	s := currentSessions()
	if s == nil {
		return 0
	}
	return s.CurrentMonthlyMiles(ctx, u)
}

func (realSessions) BonusMiles(u users.User) float32 {
	s := currentSessions()
	if s == nil {
		return 0
	}
	return s.BonusMiles(u)
}

func (realSessions) Shutdown(ctx context.Context) {
	if s := currentSessions(); s != nil {
		s.Shutdown(ctx)
	}
}

func (realSessions) SetBot(ctx context.Context, username string, isBot bool) error {
	s := currentSessions()
	if s == nil {
		return nil
	}
	return s.SetBot(ctx, username, isBot)
}
