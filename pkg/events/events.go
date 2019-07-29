package events

import (
	"fmt"
	"log"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/database"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
)

type Event struct {
	Username    string    `db:"username"`
	Event       string    `db:"event"`
	DateCreated time.Time `db:"date_created"`
}

func LogoutAll(botStart time.Time) {
	//TODO maybe don't use an Events map?
	events := []Event{}
	database.DBCon.Select(&events, "SELECT DISTINCT username from events where event='login' and date_created >= $1", botStart)
	if len(events) == 0 {
		log.Println("query resulted in no matches")
	}
	for _, event := range events {
		user := event.Username
		log.Println("logging out", user)
		LogoutIfNecessary(user)
	}
}

// LogOutIfNecessary() will create a logout event if they need one
func LogoutIfNecessary(user string) {
	// we need to check to see if the last event was a logout
	events := []Event{}
	query := fmt.Sprintf("SELECT event, date_created FROM events WHERE username='%s' AND event IN ('logout','login') ORDER BY date_created DESC LIMIT 1", user)
	database.DBCon.Select(&events, query)
	if len(events) == 0 {
		// no login/logout events for user
		log.Println("no login/logout events for", user, "how did we get here?")
		return
	}
	event := events[0]
	if event.Event == "login" {
		// no output if they are an ignored user
		if !helpers.UserIsIgnored(user) {
			// include the duration they were logged in
			loggedInDur := time.Now().Sub(event.DateCreated)
			// last event was a login, so log them out
			log.Printf("logging out %s (%s)", user, loggedInDur)
		}
		Logout(user)
		return
	}
	// nothing to be done
	return
}

// LoginIfNecessary() will create a login event if there should already be one
func LoginIfNecessary(user string) {
	events := []Event{}
	query := fmt.Sprintf("SELECT event, date_created FROM events WHERE username='%s' AND event IN ('logout','login') ORDER BY date_created DESC LIMIT 1", user)
	database.DBCon.Select(&events, query)
	if len(events) == 0 {
		// no output if they are an ignored user
		if !helpers.UserIsIgnored(user) {
			// no login/logout events for user
			log.Println("logging in", user)
		}
		Login(user)
		return
	}
	event := events[0]
	if event.Event == "logout" {
		// no output if they are an ignored user
		if !helpers.UserIsIgnored(user) {
			// last event was a logout, so log them in
			log.Println("logging in", user)
		}
		Login(user)
		return
	}
	// nothing to be done
	return
}

//TODO: make this private?
func Login(user string) {
	tx := database.DBCon.MustBegin()
	tx.MustExec("INSERT INTO events (username, event) VALUES ($1, $2)", user, "login")
	tx.Commit()
}

//TODO: make this private?
func Logout(user string) {
	tx := database.DBCon.MustBegin()
	tx.MustExec("INSERT INTO events (username, event) VALUES ($1, $2)", user, "logout")
	tx.Commit()
}
