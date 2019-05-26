package events

import (
	"fmt"
	"log"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/database"
)

type Event struct {
	Username    string    `db:"username"`
	Event       string    `db:"event"`
	DateCreated time.Time `db:"date_created"`
}

// LoginIfNecessary() will create a login event if there should already be one
func LoginIfNecessary(user string) {
	events := []database.Event{}
	query := fmt.Sprintf("SELECT event, date_created FROM events WHERE username='%s' AND event IN ('logout','login') ORDER BY date_created DESC LIMIT 1", user)
	database.DBCon.Select(&events, query)
	if len(events) == 0 {
		// no login/logout events for user
		log.Println("no login/logout events for", user, "so logging them in")
		Login(user)
		return
	}
	event := events[0]
	if event.Event == "logout" {
		// last event was a logout, so log them in
		log.Println("last event for", user, "was a logout, logging them in")
		Login(user)
	}
	// nothing to be done
	return
}

func Login(user string) {
	tx := database.DBCon.MustBegin()
	tx.MustExec("INSERT INTO events (username, event) VALUES ($1, $2)", user, "login")
	tx.Commit()
}

func Logout(user string) {
	tx := database.DBCon.MustBegin()
	tx.MustExec("INSERT INTO events (username, event) VALUES ($1, $2)", user, "logout")
	tx.Commit()
}
