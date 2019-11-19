package users

import (
	"log"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
)

// LoggedIn is a map that contains all the currently logged-in users,
// mapped to their login time
var LoggedIn map[string]time.Time

// ShutDown loops through all of the logged-in users and logs them out
func Shutdown() {
	log.Println("these were the logged-in users")
	spew.Dump(LoggedIn)
	for username, _ := range LoggedIn {
		user := Find(username)
		user.logout()
	}
}

// FindInSession searches the current session for the user
func FindInSession(username string) User {
	if _, ok := LoggedIn[username]; ok {
		//TODO: maybe don't return a User here?
		return Find(username)
	}
	return nilUser
}

func PrintCurrentSession() {
	usernames := make([]string, 0, len(LoggedIn))
	for username := range LoggedIn {
		usernames = append(usernames, username)
	}
	log.Printf("Currently logged in: %s", strings.Join(usernames, ", "))
}
