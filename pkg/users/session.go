package users

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/dmerrick/danalol-stream/pkg/twitch"
)

// LoggedIn is a map that contains all the currently logged-in users,
// mapped to their login time
// var LoggedIn map[string]time.Time
var LoggedIn = make(map[string]time.Time)

func UpdateSession() {
	//TODO: move this to a separate CRON task
	fmt.Println("updating viewers")
	twitch.UpdateViewers()

	currentChatters := twitch.Chatters()
	spew.Dump(currentChatters)

	// log out the people who arent present
	for loggedInUser, _ := range LoggedIn {
		//TODO: consider changing currentChatters to a map to make this faster
		for _, chatter := range currentChatters {
			if chatter == loggedInUser {
				// they're logged in and a current chatter, do nothing
				break
			} else {
				// they're logged in and NOT a current chatter, so log them out
				LogoutIfNecessary(loggedInUser)
				break
			}
		}

	}

	// log in everybody else
	//TODO: this could get slow, maybe make a list of users that need to be logged in?
	for _, chatter := range currentChatters {
		LoginIfNecessary(chatter)
	}
}

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
