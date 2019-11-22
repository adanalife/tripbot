package users

import (
	"errors"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/dmerrick/danalol-stream/pkg/events"
	"github.com/dmerrick/danalol-stream/pkg/helpers"

	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/dmerrick/danalol-stream/pkg/twitch"
	"github.com/logrusorgru/aurora"
)

//TODO: consider moving this whole thing elsewhere (to background perhaps?)

// LoggedIn is a map that contains all the currently logged-in users,
// mapping their username to a User
var LoggedIn = make(map[string]User)

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
func LoginIfNecessary(username string) {
	// check if the user is currently logged in
	if isLoggedIn(username) {
		return
	}
	// they weren't logged in, so note in the DB
	login(username)
}

// LogoutIfNecessary will log out the user if it finds them in the session
func LogoutIfNecessary(username string) {
	if isLoggedIn(username) {
		user := LoggedIn[username]
		user.logout()
		return
	}
	log.Println("hmm, LogoutIfNecessary() called and user not logged in:", aurora.Magenta(username))
}

// login will record the users presence in the DB
//TODO: do we want to make a DB update here? we could do it on logout()
func login(username string) {
	now := time.Now()

	user := FindOrCreate(username)
	// increment the number of visits
	user.NumVisits = user.NumVisits + 1
	// set the login time
	user.LoggedIn = now
	// update the last seen date
	user.LastSeen = now
	user.save()

	// raise an error if a user is supposed to be a bot
	if helpers.UserIsIgnored(username) && !user.IsBot {
		terrors.Log(errors.New("user should be bot"), username)
	}

	// add them to the session
	LoggedIn[username] = user

	// create a login event as well
	events.Login(username)
}

// User.logout() removes the user from the list of currently-logged in users,
// and updates the DB with their most up-to-date values
func (u User) logout() {
	log.Println("logging out", aurora.Magenta(u.Username))

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

// logout() removes the user from the list of currently-logged in users,
// and updates the DB with their most up-to-date values
// func logout(username string) {
// 	log.Println("logging out", aurora.Magenta(username))

// 	now := time.Now()
// 	user := Find(username)

// 	// update miles
// 	user.Miles = user.CurrentMiles()
// 	// update the last seen date
// 	user.LastSeen = now
// 	// store the user in the db
// 	user.save()

// 	// create a login event as well
// 	events.Logout(username)

// 	// remove them from the session
// 	delete(LoggedIn, username)
// }

// isLoggedIn checks if the user is currently logged in
func isLoggedIn(username string) bool {
	if _, ok := LoggedIn[username]; ok {
		return true
	}
	return false
}

// ShutDown loops through all of the logged-in users and logs them out
func Shutdown() {
	log.Println("these were the logged-in users")
	spew.Dump(LoggedIn)
	for _, user := range LoggedIn {
		user.logout()
	}
}

// PrintCurrentSession simply prints info about the current session
func PrintCurrentSession() {
	bots := 0

	// create a list of only usernames, and sort it
	usernames := make([]string, 0, len(LoggedIn))
	for username, _ := range LoggedIn {
		usernames = append(usernames, username)
	}
	sort.Sort(sort.StringSlice(usernames))

	// now loop over the sorted names and colorize them
	coloredUsernames := make([]string, 0, len(usernames))
	for _, username := range usernames {
		user := LoggedIn[username]
		if user.IsBot {
			bots = bots + 1
		}
		// add the colored username to the list
		coloredUsernames = append(coloredUsernames, user.String())
	}
	log.Println("there are",
		twitch.ChatterCount(), "people in chat,",
		aurora.Cyan(len(LoggedIn)-bots), "humans, and",
		aurora.Gray(15, bots), "bots",
	)

	log.Printf("Currently logged in: %s", strings.Join(coloredUsernames, ", "))
}
