package users

import (
	"fmt"
	"strconv"
	"strings"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	config "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/jmoiron/sqlx"
	"github.com/logrusorgru/aurora"
)

var Leaderboard [][]string
var initLeaderboardSize = 25
var maxLeaderboardSize = 50

// Leaderboard creates a leaderboard
func InitLeaderboard() {
	users := []User{}

	ignoredUsers := append(config.IgnoredUsers, strings.ToLower(c.Conf.ChannelName))
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
		Leaderboard = append(Leaderboard, pair)
	}
}

func UpdateLeaderboard() {
	for _, user := range LoggedIn {
		// skip adding this user if they're a bot or ignored
		if user.IsBot || helpers.UserIsIgnored(user.Username) || helpers.UserIsAdmin(user.Username) {
			continue
		}
		insertIntoLeaderboard(*user)
	}
	// truncate Leaderboard if it gets too big
	if len(Leaderboard) > maxLeaderboardSize {
		Leaderboard = Leaderboard[:maxLeaderboardSize]
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

	for i, pair := range Leaderboard {
		val := strToFloat32(pair[1])
		// see if our miles are higher
		if miles >= val {
			milesStr := fmt.Sprintf("%.1f", miles)
			newPair := []string{user.Username, milesStr}

			// insert into Leaderboard
			// https://github.com/golang/go/wiki/SliceTricks#insert
			Leaderboard = append(Leaderboard[:i], append([][]string{newPair}, Leaderboard[i:]...)...)
			return
		}
	}
}

// removeFromLeaderboard searches the Leaderboard for
// a username and removes it
func removeFromLeaderboard(username string) {
	for i, pair := range Leaderboard {
		if pair[0] == username {
			// delete from Leaderboard
			// https://github.com/golang/go/wiki/SliceTricks#delete
			Leaderboard = append(Leaderboard[:i], Leaderboard[i+1:]...)
			return
		}
	}
}

// this was used for development
func printLeaderboard() {
	for i, pair := range Leaderboard {
		fmt.Printf("%d: %s - %s\n", i+1, pair[1], aurora.Magenta(pair[0]))
	}
}

// LeaderboardContent creates the content for the leaderboard onscreen
func LeaderboardContent() string {
	var output string
	output = "Odometer Leaderboard\n"

	size := 5
	if len(Leaderboard) < size {
		size = len(Leaderboard)
	}
	leaderboard := Leaderboard[:size]

	for _, score := range leaderboard {
		output = output + fmt.Sprintf("%s miles: %s\n", score[1], score[0])
	}

	return output
}
