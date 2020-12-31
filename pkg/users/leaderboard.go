package users

import (
	"fmt"
	"strconv"
	"strings"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/jmoiron/sqlx"
	"github.com/logrusorgru/aurora"
)

var LifetimeMilesLeaderboard [][]string
var initLeaderboardSize = 25
var maxLeaderboardSize = 50

// LifetimeMilesLeaderboard creates a leaderboard
func InitLeaderboard() {
	users := []User{}

	ignoredUsers := append(c.IgnoredUsers, strings.ToLower(c.Conf.ChannelName))
	// we use MySQL-style ? bindvars instead of postgres ones here
	// because that's what sqlx wants for In()
	q := `SELECT * FROM users WHERE miles != 0 AND is_bot = false AND username NOT IN (?) ORDER BY miles DESC LIMIT ?`
	query, args, err := sqlx.In(q, ignoredUsers, initLeaderboardSize)
	if err != nil {
		terrors.Log(err, "error generating query")
	}
	query = database.Connection().Rebind(query)
	err = database.Connection().Select(&users, query, args...)
	if err != nil {
		terrors.Log(err, "error generating query")
	}

	for _, user := range users {
		miles := fmt.Sprintf("%.1f", user.Miles)
		pair := []string{user.Username, miles}
		LifetimeMilesLeaderboard = append(LifetimeMilesLeaderboard, pair)
	}
}

func UpdateLeaderboard() {
	for _, user := range LoggedIn {
		// skip adding this user if they're a bot or ignored
		if user.IsBot || c.UserIsIgnored(user.Username) || c.UserIsAdmin(user.Username) {
			continue
		}
		insertIntoLeaderboard(*user)
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

func insertIntoLeaderboard(user User) {
	// first we remove this user from the board
	removeFromLeaderboard(user.Username)

	// get the current miles as a float
	miles := user.CurrentMiles()

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
func LeaderboardContent() string {
	var output string
	output = "Odometer LifetimeMilesLeaderboard\n"

	size := 5
	if len(LifetimeMilesLeaderboard) < size {
		size = len(LifetimeMilesLeaderboard)
	}
	leaderboard := LifetimeMilesLeaderboard[:size]

	for _, score := range leaderboard {
		output = output + fmt.Sprintf("%s miles: %s\n", score[1], score[0])
	}

	return output
}
