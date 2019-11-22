package events

import (
	"log"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/database"
	"github.com/logrusorgru/aurora"
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
		log.Println("logging out", aurora.Magenta(user))
		LogoutIfNecessary(user)
	}
}

func Login(user string) {
	if config.ReadOnly && config.Verbose {
		log.Printf("Not logging in %s because we're in read-only mode", aurora.Magenta(user))
		return
	}
	tx := database.DBCon.MustBegin()
	tx.MustExec("INSERT INTO events (username, event) VALUES ($1, $2)", user, "login")
	tx.Commit()
}

func Logout(user string) {
	if config.ReadOnly && config.Verbose {
		log.Printf("Not logging out %s because we're in read-only mode", aurora.Magenta(user))
		return
	}
	tx := database.DBCon.MustBegin()
	tx.MustExec("INSERT INTO events (username, event) VALUES ($1, $2)", user, "logout")
	tx.Commit()
}
