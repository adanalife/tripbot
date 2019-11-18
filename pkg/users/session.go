package users

import (
	"log"
	"strings"

	"github.com/davecgh/go-spew/spew"
)

// LoggedIn is a slice that contains all the currently logged-in users
var LoggedIn []User

// ShutDown loops through all of the logged-in users and logs them out
func Shutdown() {
	log.Println("these were the logged-in users")
	spew.Dump(LoggedIn)
	for _, u := range LoggedIn {
		u.logout()
	}
}

// FindInSession searches the current session for the user
func FindInSession(username string) User {
	for _, loggedInUser := range LoggedIn {
		if username == loggedInUser.Username {
			return loggedInUser
		}
	}
	return nilUser
}

func PrintCurrentSession() {
	var usernames []string
	for _, user := range LoggedIn {
		usernames = append(usernames, user.Username)
	}
	log.Printf("Currently logged in: %s", strings.Join(usernames, ", "))
}
