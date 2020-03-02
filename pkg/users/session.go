package users

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/events"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/hako/durafmt"

	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/dmerrick/danalol-stream/pkg/twitch"
	mytwitch "github.com/dmerrick/danalol-stream/pkg/twitch"
	"github.com/logrusorgru/aurora"
)

//TODO: consider moving this whole thing elsewhere (to background perhaps?)

// LoggedIn is a map that contains all the currently logged-in users,
// mapping their username to a User
var LoggedIn = make(map[string]*User)

// UpdateSession will use the data from the Twitch API to maintain a list
// of currently-logged-in users
func UpdateSession() {
	// fetch the latest chatters from Twitch
	twitch.UpdateChatters()
	currentChatters := twitch.Chatters()

	// log out the people who arent present
	for username, user := range LoggedIn {
		if _, ok := currentChatters[username]; ok {
			// they're logged in and a current chatter, do nothing
			continue
		} else {
			// they're logged in and NOT a current chatter, so log them out
			user.logout()
			continue
		}
	}

	// log in everybody else
	//TODO: this could get slow, maybe make a list of users that need to be logged in?
	for chatter, _ := range currentChatters {
		LoginIfNecessary(chatter)
	}

	// PrintCurrentSession()
	// twitch.PrintCurrentChatters()
}

// LoginIfNecessary checks the list of currently-logged in users and will
// run login() if this user isn't currently logged in
func LoginIfNecessary(username string) *User {
	// check if the user is currently logged in
	if isLoggedIn(username) {
		return LoggedIn[username]
	}
	// they weren't logged in, so note in the DB
	return login(username)
}

// LogoutIfNecessary will log out the user if it finds them in the session
func LogoutIfNecessary(username string) {
	if isLoggedIn(username) {
		user := *LoggedIn[username]
		user.logout()
	}
}

// login will record the users presence in the DB
//TODO: do we want to make a DB update here? we could do it on logout()
func login(username string) *User {
	now := time.Now()

	user := FindOrCreate(username)
	// increment the number of visits
	user.NumVisits = user.NumVisits + 1
	// set the login time
	user.LoggedIn = now
	// update the last seen date
	user.LastSeen = now
	// set their last command date yesterday
	user.lastCmd = now.AddDate(0, 0, -1)
	user.save()

	// raise an error if a user is supposed to be a bot
	if helpers.UserIsIgnored(username) && !user.IsBot {
		terrors.Log(errors.New("user should be bot"), username)
	}

	// just a silly message to confirm subscriber feature is working
	if mytwitch.UserIsSubscriber(username) {
		msg := fmt.Sprintf("subscriber %s logged in!", username)
		log.Println(aurora.Magenta(msg))
	}

	// add them to the session
	LoggedIn[username] = &user

	// create a login event as well
	events.Login(username)

	return &user
}

// User.logout() removes the user from the list of currently-logged in users,
// and updates the DB with their most up-to-date values
func (u User) logout() {

	// print logout message if they're human
	if !u.IsBot {
		loggedInDur := time.Now().Sub(u.LoggedIn)
		prettyDur := durafmt.ParseShort(loggedInDur)
		dur := fmt.Sprintf("(%s)", aurora.Green(prettyDur))
		log.Println("logging out", u, dur)
	}

	now := time.Now()
	// update miles
	u.Miles = u.CurrentMiles()
	// update the last seen date
	u.LastSeen = now
	// store the user in the db
	u.save()

	// create a login event as well
	events.Logout(u.Username)

	// remove them from the session
	delete(LoggedIn, u.Username)
}

// isLoggedIn checks if the user is currently logged in
func isLoggedIn(username string) bool {
	if _, ok := LoggedIn[username]; ok {
		return true
	}
	return false
}

// ShutDown loops through all of the logged-in users and logs them out
func Shutdown() {
	if config.Verbose {
		log.Println("these were the logged-in users")
		spew.Dump(LoggedIn)
	}
	for _, user := range LoggedIn {
		user.logout()
	}
}

// GiveEveryoneMiles gives all logged-in users miles
func GiveEveryoneMiles(gift float32) {
	log.Println(aurora.Green("giving all logged-in users gift miles"))
	for _, user := range LoggedIn {
		user.Miles += gift
	}
}

// sortedUsernameList creates a list of only usernames, and sort it
func sortedUsernameList() []string {
	usernames := make([]string, 0, len(LoggedIn))
	for username, _ := range LoggedIn {
		usernames = append(usernames, username)
	}
	sort.Sort(sort.StringSlice(usernames))
	return usernames
}

// colorizeUsernames loops over the sorted names and colorizes them
func colorizeUsernames(usernames []string) []string {
	coloredUsernames := make([]string, 0, len(usernames))
	for _, username := range usernames {
		user := *LoggedIn[username]
		if user.IsBot {
			// don't add them to the output
			continue
		}
		// add the colored username to the list
		coloredUsernames = append(coloredUsernames, user.String())
	}
	return coloredUsernames
}

// humans returns the users in the session who are not bots
func humans() []*User {
	var humans []*User
	for _, user := range LoggedIn {
		if !user.IsBot {
			humans = append(humans, user)
		}
	}
	return humans
}

// countHumans returns the number of humans in the session
func countHumans() int {
	return len(humans())
}

// bots returns the users in the session who are known bots
func bots() []*User {
	var bots []*User
	for _, user := range LoggedIn {
		if user.IsBot {
			bots = append(bots, user)
		}
	}
	return bots
}

// countBots returns the number of bots in the session
func countBots() int {
	return len(bots())
}

// PrintCurrentSession simply prints info about the current session
func PrintCurrentSession() {
	usernames := sortedUsernameList()
	coloredUsernames := colorizeUsernames(usernames)

	log.Println("there are",
		twitch.ChatterCount(), "users in chat,",
		aurora.Cyan(countHumans()), "humans, and",
		aurora.Gray(15, countBots()), "bots",
	)

	log.Printf("Currently logged in: %s", strings.Join(coloredUsernames, ", "))
}
