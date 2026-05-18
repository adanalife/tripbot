package users

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/logrusorgru/aurora/v3"
)

var LifetimeMilesLeaderboard [][]string
var initLeaderboardSize = 25
var maxLeaderboardSize = 50

// InitLeaderboard creates the initial leaderboard
func InitLeaderboard(ctx context.Context) {
	var users []User

	ignoredUsers := append(c.IgnoredUsers, strings.ToLower(c.Conf.ChannelName))
	result := database.GormDB().WithContext(ctx).
		Where("miles != 0 AND is_bot = false AND username NOT IN ?", ignoredUsers).
		Order("miles DESC").
		Limit(initLeaderboardSize).
		Find(&users)
	if result.Error != nil {
		terrors.Log(result.Error, "error fetching leaderboard")
	}

	for _, user := range users {
		miles := fmt.Sprintf("%.1f", user.Miles)
		pair := []string{user.Username, miles}
		LifetimeMilesLeaderboard = append(LifetimeMilesLeaderboard, pair)
	}
}

// UpdateLeaderboard rebuilds the lifetime-miles leaderboard from the
// LoggedIn map. The work is in-memory (no DB hits), so ctx only carries
// the cron-tick span for log correlation.
func UpdateLeaderboard(ctx context.Context) {
	for _, user := range LoggedIn {
		// skip adding this user if they're a bot or ignored
		if user.IsBot || c.UserIsIgnored(user.Username) || c.UserIsAdmin(user.Username) {
			continue
		}
		insertIntoLeaderboard(ctx, *user)
	}
	// truncate LifetimeMilesLeaderboard if it gets too big
	if len(LifetimeMilesLeaderboard) > maxLeaderboardSize {
		LifetimeMilesLeaderboard = LifetimeMilesLeaderboard[:maxLeaderboardSize]
	}
}

// convert the string to a float32
func strToFloat32(str string) float32 {
	value, err := strconv.ParseFloat(str, 32)
	if err != nil {
		terrors.Log(err, "error parsing float")
		return 0.0
	}
	return float32(value)
}

func insertIntoLeaderboard(ctx context.Context, user User) {
	// first we remove this user from the board
	removeFromLeaderboard(user.Username)

	// get the current miles as a float
	miles := user.CurrentMiles(ctx)

	for i, pair := range LifetimeMilesLeaderboard {
		val := strToFloat32(pair[1])
		// see if our miles are higher
		if miles >= val {
			milesStr := fmt.Sprintf("%.1f", miles)
			newPair := []string{user.Username, milesStr}

			// insert into LifetimeMilesLeaderboard
			// https://github.com/golang/go/wiki/SliceTricks#insert
			LifetimeMilesLeaderboard = append(LifetimeMilesLeaderboard[:i], append([][]string{newPair}, LifetimeMilesLeaderboard[i:]...)...)
			return
		}
	}
}

// removeFromLeaderboard searches the LifetimeMilesLeaderboard for
// a username and removes it
func removeFromLeaderboard(username string) {
	for i, pair := range LifetimeMilesLeaderboard {
		if pair[0] == username {
			// delete from LifetimeMilesLeaderboard
			// https://github.com/golang/go/wiki/SliceTricks#delete
			LifetimeMilesLeaderboard = append(LifetimeMilesLeaderboard[:i], LifetimeMilesLeaderboard[i+1:]...)
			return
		}
	}
}

// this was used for development
func printLeaderboard() {
	for i, pair := range LifetimeMilesLeaderboard {
		fmt.Printf("%d: %s - %s\n", i+1, pair[1], aurora.Magenta(pair[0]))
	}
}

// LeaderboardContent creates the content for the leaderboard onscreen
func LeaderboardContent(title string, leaderboard [][]string) string {
	var output string
	output = strings.Title(title) + "\n"

	size := 5
	if len(leaderboard) < size {
		size = len(leaderboard)
	}
	leaderboard = leaderboard[:size]

	for _, score := range leaderboard {
		output = output + fmt.Sprintf("%s (%s)\n", score[1], score[0])
	}

	return output
}
