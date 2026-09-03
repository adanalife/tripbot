package chatbot

import (
	"context"
	"fmt"
	"strings"

	"github.com/adanalife/tripbot/pkg/users"
	"gorm.io/gorm"
)

// noopSessions satisfies Sessions for tests that don't care about user
// lookups — Find reports every user as unknown, the leaderboard
// reads as empty, and Shutdown is a no-op.
type noopSessions struct{}

func (noopSessions) Find(_ context.Context, _ string) (users.User, error) {
	return users.User{}, gorm.ErrRecordNotFound
}
func (noopSessions) LifetimeLeaderboard() [][]string                             { return nil }
func (noopSessions) Shutdown(_ context.Context)                                  {}
func (noopSessions) SetBot(_ context.Context, _ string, _ bool) error            { return nil }
func (noopSessions) CurrentMiles(_ context.Context, u users.User) float32        { return u.Miles }
func (noopSessions) CurrentMonthlyMiles(_ context.Context, _ users.User) float32 { return 0 }
func (noopSessions) BonusMiles(_ users.User) float32                             { return 0 }
func (noopSessions) CorrectMiles(_ context.Context, _ string, _ float32) (float32, error) {
	return 0, nil
}
func (noopSessions) LoginIfNecessary(_ context.Context, username string) users.User {
	return users.User{Username: strings.ToLower(username)}
}
func (noopSessions) RecordPlatformUserID(_ context.Context, u users.User, _ string) users.User {
	return u
}
func (noopSessions) IsSubscriber(_ users.User) bool                            { return false }
func (noopSessions) HasCommandAvailable(_ context.Context, _ *users.User) bool { return true }
func (noopSessions) GiveEveryoneMiles(_ float32)                               {}
func (noopSessions) LoggedInCount() int                                        { return 0 }

// recordingSessions captures every call made to it so tests can assert
// the chatbot queried the expected user / leaderboard surfaces.
// Tests can stage what Find / LifetimeLeaderboard return via the
// FindResult / Leaderboard fields. All call records are appended in
// order to Calls.
type recordingSessions struct {
	Calls       []string
	FindResult  users.User
	Leaderboard [][]string
	// FindErr is the error Find will return for every call. When unset and
	// FindResult is the zero user, Find returns gorm.ErrRecordNotFound —
	// mirroring pkg/users.Find's "not found" contract.
	FindErr error
	// SetBotErr is the error SetBot will return for every call.
	SetBotErr error
	// CorrectMilesErr is the error CorrectMiles will return for every call,
	// standing in for a correction that didn't persist.
	CorrectMilesErr error
	// Miles / MonthlyMiles / Bonus stage what the miles methods return.
	Miles, MonthlyMiles, Bonus float32
	// Subscriber stages IsSubscriber; CommandUnavailable flips
	// HasCommandAvailable to false (the zero value grants, so the ordinary
	// dispatch tests run their commands).
	Subscriber, CommandUnavailable bool
	// LoggedIn stages LoggedInCount; Gifts records every GiveEveryoneMiles.
	LoggedIn int
	Gifts    []float32
}

func (r *recordingSessions) LoginIfNecessary(_ context.Context, username string) users.User {
	r.Calls = append(r.Calls, fmt.Sprintf("LoginIfNecessary(%q)", username))
	if r.FindResult.Username == username {
		return r.FindResult
	}
	return users.User{Username: username}
}

func (r *recordingSessions) RecordPlatformUserID(_ context.Context, u users.User, platformUserID string) users.User {
	r.Calls = append(r.Calls, fmt.Sprintf("RecordPlatformUserID(%q, %q)", u.Username, platformUserID))
	u.PlatformUserID = platformUserID
	return u
}

func (r *recordingSessions) IsSubscriber(u users.User) bool {
	r.Calls = append(r.Calls, fmt.Sprintf("IsSubscriber(%q)", u.Username))
	return r.Subscriber
}

func (r *recordingSessions) HasCommandAvailable(_ context.Context, u *users.User) bool {
	r.Calls = append(r.Calls, fmt.Sprintf("HasCommandAvailable(%q)", u.Username))
	return !r.CommandUnavailable
}

func (r *recordingSessions) GiveEveryoneMiles(gift float32) {
	r.Calls = append(r.Calls, fmt.Sprintf("GiveEveryoneMiles(%g)", gift))
	r.Gifts = append(r.Gifts, gift)
}

func (r *recordingSessions) LoggedInCount() int {
	r.Calls = append(r.Calls, "LoggedInCount")
	return r.LoggedIn
}

func (r *recordingSessions) Find(_ context.Context, username string) (users.User, error) {
	r.Calls = append(r.Calls, fmt.Sprintf("Find(%q)", username))
	if r.FindErr != nil {
		return users.User{}, r.FindErr
	}
	if r.FindResult.ID == 0 {
		return users.User{}, gorm.ErrRecordNotFound
	}
	return r.FindResult, nil
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

func (r *recordingSessions) CorrectMiles(_ context.Context, username string, delta float32) (float32, error) {
	r.Calls = append(r.Calls, fmt.Sprintf("CorrectMiles(%q, %g)", username, delta))
	if r.CorrectMilesErr != nil {
		return 0, r.CorrectMilesErr
	}
	return r.Miles, nil
}
