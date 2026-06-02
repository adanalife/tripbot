package chatbot

import (
	"context"

	"github.com/adanalife/tripbot/pkg/users"
)

// Sessions is the subset of the pkg/users surface that chatbot commands
// depend on at command-time (user lookups, lifetime-leaderboard reads,
// miles computations, graceful shutdown of in-memory session state). Tests
// inject a fake; production uses the realSessions adapter built by
// NewSessionsAdapter. Mirrors the Onscreens/VLC/Video/IRC injection pattern.
//
// The IRC-side session lifecycle (LoginIfNecessary / LogoutIfNecessary) and the
// follower/subscriber + login-count reads are intentionally NOT on this
// interface — they're called from the inbound handlers (HandleMessage / Join /
// Part) and dispatch's access check, which reach the concrete *users.Sessions
// through App.UserSessions directly. Keeping them off this interface keeps the
// command-time fake surface (noopSessions / recordingSessions) minimal.
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

// realSessions delegates to its *users.Sessions, plus pkg/users' standalone DB
// helper (Find, which is not session state). cmd/tripbot builds it around the
// process-wide *users.Sessions via NewSessionsAdapter so commands read the same
// session state the IRC handlers mutate. s is nil in New()'s default adapter —
// the brief startup window before cmd assigns App.Sessions, and the defaultApp
// test fixture — so the nil guards below cover that. Tests inject their own
// Sessions fake rather than realSessions, so the guards only ever fire
// pre-install.
type realSessions struct{ s *users.Sessions }

// NewSessionsAdapter builds the production Sessions adapter around s. cmd/tripbot
// assigns the result onto App.Sessions once Sessions is constructed.
func NewSessionsAdapter(s *users.Sessions) Sessions { return realSessions{s: s} }

func (r realSessions) Find(ctx context.Context, username string) users.User {
	return users.Find(ctx, username)
}

func (r realSessions) LifetimeLeaderboard() [][]string {
	if r.s == nil {
		return nil
	}
	return r.s.LifetimeLeaderboard()
}

func (r realSessions) CurrentMiles(ctx context.Context, u users.User) float32 {
	if r.s == nil {
		return u.Miles
	}
	return r.s.CurrentMiles(ctx, u)
}

func (r realSessions) CurrentMonthlyMiles(ctx context.Context, u users.User) float32 {
	if r.s == nil {
		return 0
	}
	return r.s.CurrentMonthlyMiles(ctx, u)
}

func (r realSessions) BonusMiles(u users.User) float32 {
	if r.s == nil {
		return 0
	}
	return r.s.BonusMiles(u)
}

func (r realSessions) Shutdown(ctx context.Context) {
	if r.s != nil {
		r.s.Shutdown(ctx)
	}
}

func (r realSessions) SetBot(ctx context.Context, username string, isBot bool) error {
	if r.s == nil {
		return nil
	}
	return r.s.SetBot(ctx, username, isBot)
}
