package users

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
)

var initLeaderboardSize = 25
var maxLeaderboardSize = 50

// fetchLeaderboard reads the top users by stored lifetime miles, scoped to
// this instance's platform, excluding bots and the channel owner.
func fetchLeaderboard(ctx context.Context, limit int) ([]User, error) {
	var users []User
	result := database.GormDB().WithContext(ctx).
		Where("platform = ? AND miles != 0 AND is_bot = false AND username != ?", c.Conf.Platform, strings.ToLower(c.Conf.ChannelName)).
		Order("miles DESC").
		Limit(limit).
		Find(&users)
	return users, result.Error
}

// InitLeaderboard creates the initial leaderboard
func (s *Sessions) InitLeaderboard(ctx context.Context) {
	users, err := fetchLeaderboard(ctx, initLeaderboardSize)
	if err != nil {
		slog.ErrorContext(ctx, "error fetching leaderboard", "err", err)
	}
	s.lifetimeLeaderboard = toPairs(users)
}

// UpdateLeaderboard rebuilds the lifetime-miles leaderboard from the users
// table, overlaying in-progress session miles for anyone currently logged in
// so lurkers' numbers keep ticking between logouts. Replaces the cached slice
// wholesale; before this it was rebuilt in-memory from logged-in users only,
// which drifted from the DB after boot.
func (s *Sessions) UpdateLeaderboard(ctx context.Context) {
	users, err := fetchLeaderboard(ctx, maxLeaderboardSize)
	if err != nil {
		slog.ErrorContext(ctx, "error fetching leaderboard", "err", err)
		return
	}
	for i, user := range users {
		if live, ok := s.loggedIn[user.Username]; ok {
			users[i].Miles = s.CurrentMiles(ctx, *live)
		}
	}
	// ponytail: a logged-in user whose stored miles sit just below the top-50
	// cutoff won't appear until logout — same class of miss as the old
	// in-memory rebuild.
	sort.SliceStable(users, func(i, j int) bool { return users[i].Miles > users[j].Miles })
	s.lifetimeLeaderboard = toPairs(users)
}

// toPairs formats users as the [username, miles] string pairs the leaderboard
// consumers render, skipping admin accounts (the DB query already excludes
// bots and the channel owner).
func toPairs(users []User) [][]string {
	pairs := make([][]string, 0, len(users))
	for _, user := range users {
		if c.UserIsAdmin(user.Username) {
			continue
		}
		pairs = append(pairs, []string{user.Username, fmt.Sprintf("%.1f", user.Miles)})
	}
	return pairs
}

// LifetimeLeaderboard returns the cached lifetime-miles leaderboard (a slice of
// [username, miles] pairs), hydrated by InitLeaderboard and rebuilt by
// UpdateLeaderboard.
func (s *Sessions) LifetimeLeaderboard() [][]string { return s.lifetimeLeaderboard }
