package chatbot

import (
	"context"
	"errors"
	"strings"

	"github.com/adanalife/tripbot/pkg/users"
)

// Sessions is the pkg/users surface the chatbot depends on: command-time
// queries (user lookups, lifetime-leaderboard reads, miles computations,
// graceful shutdown of in-memory session state) plus the inbound-side session
// lifecycle and access-check reads the handlers and dispatch need. Tests
// inject a fake; production uses the realSessions adapter built by
// NewSessionsAdapter. Mirrors the Onscreens/Playout/Video/IRC injection pattern.
type Sessions interface {
	// Find looks up a user by username. Returns gorm.ErrRecordNotFound for
	// an unknown user (mirrors pkg/users.Find's contract); any other error
	// is a real DB failure.
	Find(ctx context.Context, username string) (users.User, error)
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
	// CorrectMiles applies a manual miles delta (may be negative) to a user's
	// lifetime total and current monthly scoreboard, persisting immediately,
	// and returns the new lifetime total. Backs !givemiles. An error means
	// nothing was persisted and the returned total is meaningless — don't
	// report it, and don't record a correction event.
	CorrectMiles(ctx context.Context, username string, delta float32) (float32, error)
	// Shutdown logs out every in-memory session, flushing each user's
	// state to the DB. Called by !shutdown before the process exits.
	Shutdown(ctx context.Context)
	// SetBot flips users.is_bot for a username. Returns
	// gorm.ErrRecordNotFound if the user doesn't exist.
	SetBot(ctx context.Context, username string, isBot bool) error

	// LoginIfNecessary returns the live session entry for username, logging
	// them in (and refreshing their users row) first when they have none. The
	// inbound handlers call it on platforms that persist users.
	LoginIfNecessary(ctx context.Context, username string) users.User
	// RecordPlatformUserID persists u's platform-side ID and keeps the live
	// session entry in step, returning the updated User.
	RecordPlatformUserID(ctx context.Context, u users.User, platformUserID string) users.User
	// IsSubscriber reports whether the platform currently lists u as a
	// subscriber. Backs the subscriber gate in dispatch's access check.
	IsSubscriber(u users.User) bool
	// HasCommandAvailable reports whether u may run a gated command now:
	// followers always, everyone else once a day. Mutates u's last-command
	// timestamp when it grants.
	HasCommandAvailable(ctx context.Context, u *users.User) bool
	// GiveEveryoneMiles grants gift miles to every logged-in viewer.
	GiveEveryoneMiles(gift float32)
	// LoggedInCount is the number of viewers with a live session.
	LoggedInCount() int
}

// realSessions delegates to its *users.Sessions, plus pkg/users' standalone DB
// helper (Find, which is not session state). cmd/tripbot builds it around the
// process-wide *users.Sessions via NewSessionsAdapter so commands read the same
// session state the IRC handlers mutate. s is nil in New()'s default adapter
// until cmd assigns App.Sessions, so the nil guards below cover that brief
// startup window. Tests inject their own Sessions fake rather than realSessions,
// so the guards only ever fire pre-install.
type realSessions struct {
	platform string
	s        *users.Sessions
}

// NewSessionsAdapter builds the production Sessions adapter around s, scoped
// to the platform whose users it looks up. cmd/tripbot assigns the result onto
// App.Sessions once Sessions is constructed.
func NewSessionsAdapter(platform string, s *users.Sessions) Sessions {
	return realSessions{platform: platform, s: s}
}

func (r realSessions) Find(ctx context.Context, username string) (users.User, error) {
	return users.Find(ctx, r.platform, username)
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

func (r realSessions) CorrectMiles(ctx context.Context, username string, delta float32) (float32, error) {
	if r.s == nil {
		// The read-only guards above can fall back to a zero value, but a
		// correction that never reached a session can't report a total.
		return 0, errors.New("chatbot: sessions adapter has no session state")
	}
	return r.s.CorrectMiles(ctx, username, delta)
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

func (r realSessions) LoginIfNecessary(ctx context.Context, username string) users.User {
	if r.s == nil {
		// No session state to log into yet: hand back the transient shape the
		// gateway platforms use, so the message still reaches the command path.
		return users.User{Username: strings.ToLower(username)}
	}
	return r.s.LoginIfNecessary(ctx, username)
}

func (r realSessions) RecordPlatformUserID(ctx context.Context, u users.User, platformUserID string) users.User {
	if r.s == nil {
		return u
	}
	return r.s.RecordPlatformUserID(ctx, u, platformUserID)
}

func (r realSessions) IsSubscriber(u users.User) bool {
	return r.s != nil && r.s.IsSubscriber(u)
}

func (r realSessions) HasCommandAvailable(ctx context.Context, u *users.User) bool {
	if r.s == nil {
		// Without session state there is no command history to rate-limit
		// against, and nothing is connected to chat before cmd assigns one.
		return true
	}
	return r.s.HasCommandAvailable(ctx, u)
}

func (r realSessions) GiveEveryoneMiles(gift float32) {
	if r.s != nil {
		r.s.GiveEveryoneMiles(gift)
	}
}

func (r realSessions) LoggedInCount() int {
	if r.s == nil {
		return 0
	}
	return r.s.LoggedInCount()
}
