package users

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	"github.com/logrusorgru/aurora/v3"
)

var initLeaderboardSize = 25
var maxLeaderboardSize = 50

// InitLeaderboard creates the initial leaderboard
func (s *Sessions) InitLeaderboard(ctx context.Context) {
	var users []User

	result := database.GormDB().WithContext(ctx).
		Where("miles != 0 AND is_bot = false AND username != ?", strings.ToLower(c.Conf.ChannelName)).
		Order("miles DESC").
		Limit(initLeaderboardSize).
		Find(&users)
	if result.Error != nil {
		slog.ErrorContext(ctx, "error fetching leaderboard", "err", result.Error)
	}

	for _, user := range users {
		miles := fmt.Sprintf("%.1f", user.Miles)
		pair := []string{user.Username, miles}
		s.lifetimeLeaderboard = append(s.lifetimeLeaderboard, pair)
	}
}

// UpdateLeaderboard rebuilds the lifetime-miles leaderboard from the
// session's logged-in users. The work is in-memory (no DB hits), so ctx
// only carries the cron-tick span for log correlation.
func (s *Sessions) UpdateLeaderboard(ctx context.Context) {
	for _, user := range s.loggedIn {
		// skip adding this user if they're a bot or the channel owner
		if user.IsBot || c.UserIsAdmin(user.Username) {
			continue
		}
		s.insertIntoLeaderboard(ctx, *user)
	}
	// truncate the leaderboard if it gets too big
	if len(s.lifetimeLeaderboard) > maxLeaderboardSize {
		s.lifetimeLeaderboard = s.lifetimeLeaderboard[:maxLeaderboardSize]
	}
}

// convert the string to a float32
func strToFloat32(ctx context.Context, str string) float32 {
	value, err := strconv.ParseFloat(str, 32)
	if err != nil {
		slog.ErrorContext(ctx, "error parsing float", "err", err)
		return 0.0
	}
	return float32(value)
}

func (s *Sessions) insertIntoLeaderboard(ctx context.Context, user User) {
	// first we remove this user from the board
	s.removeFromLeaderboard(user.Username)

	// get the current miles as a float
	miles := user.CurrentMiles(ctx)

	for i, pair := range s.lifetimeLeaderboard {
		val := strToFloat32(ctx, pair[1])
		// see if our miles are higher
		if miles >= val {
			milesStr := fmt.Sprintf("%.1f", miles)
			newPair := []string{user.Username, milesStr}

			// insert into the leaderboard
			// https://github.com/golang/go/wiki/SliceTricks#insert
			s.lifetimeLeaderboard = append(s.lifetimeLeaderboard[:i], append([][]string{newPair}, s.lifetimeLeaderboard[i:]...)...)
			return
		}
	}
}

// removeFromLeaderboard searches the leaderboard for a username and removes it
func (s *Sessions) removeFromLeaderboard(username string) {
	for i, pair := range s.lifetimeLeaderboard {
		if pair[0] == username {
			// delete from the leaderboard
			// https://github.com/golang/go/wiki/SliceTricks#delete
			s.lifetimeLeaderboard = append(s.lifetimeLeaderboard[:i], s.lifetimeLeaderboard[i+1:]...)
			return
		}
	}
}

// this was used for development
func (s *Sessions) printLeaderboard() {
	for i, pair := range s.lifetimeLeaderboard {
		fmt.Printf("%d: %s - %s\n", i+1, pair[1], aurora.Magenta(pair[0]))
	}
}

// LifetimeMilesLeaderboard returns the cached lifetime-miles leaderboard from
// the default session. Was a package-level slice; now an accessor over the
// session-owned state.
func LifetimeMilesLeaderboard() [][]string { return defaultSessions.lifetimeLeaderboard }

func InitLeaderboard(ctx context.Context) { defaultSessions.InitLeaderboard(ctx) }

func UpdateLeaderboard(ctx context.Context) { defaultSessions.UpdateLeaderboard(ctx) }
