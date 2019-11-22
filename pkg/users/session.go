package users

import (
	"log"
	"sort"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/dmerrick/danalol-stream/pkg/events"
	"github.com/dmerrick/danalol-stream/pkg/twitch"
	"github.com/logrusorgru/aurora"
)

//TODO: consider moving this whole thing elsewhere (to background perhaps?)

// LoggedIn is a map that contains all the currently logged-in users,
// mapped to their login time
// var LoggedIn map[string]time.Time
var LoggedIn = make(map[string]time.Time)

// UpdateSession will use the data from the Twitch API to maintain a list
// of currently-logged-in users
func UpdateSession() {
	// fetch the latest chatters from Twitch
	twitch.UpdateChatters()
	currentChatters := twitch.Chatters()

	// log out the people who arent present
	for loggedInUser, _ := range LoggedIn {
		if _, ok := currentChatters[loggedInUser]; ok {
			// they're logged in and a current chatter, do nothing
			continue
		} else {
			// they're logged in and NOT a current chatter, so log them out
			logout(loggedInUser)
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
		logout(username)
		return
	}
	log.Println("hmm, LogoutIfNecessary() called and user not logged in:", aurora.Magenta(username))
}

// login will record the users presence in the DB
//TODO: do we want to make a DB update here? we could do it on logout()
func login(username string) {
	now := time.Now()
	user := FindOrCreate(username)

	LoggedIn[username] = now
	// increment the number of visits
	user.NumVisits = user.NumVisits + 1
	// update the last seen date
	user.LastSeen = now
	user.save()

	// create a login event as well
	events.Login(username)
}

// logout() removes the user from the list of currently-logged in users,
// and updates the DB with their most up-to-date values
func logout(username string) {
	log.Println("logging out", aurora.Magenta(username))

	now := time.Now()
	user := Find(username)

	// update miles
	user.Miles = user.CurrentMiles()
	// update the last seen date
	user.LastSeen = now
	// store the user in the db
	spew.Dump(user)
	user.save()

	// create a login event as well
	events.Logout(username)

	// remove them from the session
	delete(LoggedIn, username)
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
	log.Println("these were the logged-in users")
	spew.Dump(LoggedIn)
	for username, _ := range LoggedIn {
		logout(username)
	}
}

// PrintCurrentSession simply prints info about the current session
func PrintCurrentSession() {
	log.Println("there are", aurora.Cyan(twitch.ChatterCount()), "people in chat and", aurora.Cyan(len(LoggedIn)), "in the session")

	usernames := make([]string, 0, len(LoggedIn))
	for username := range LoggedIn {
		usernames = append(usernames, aurora.Magenta(username).String())
	}
	sort.Sort(sort.StringSlice(usernames))
	log.Printf("Currently logged in: %s", strings.Join(usernames, ", "))
}
