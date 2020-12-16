package events

import (
	"log"
	"time"

	"github.com/adanalife/tripbot/internal/config"
	"github.com/adanalife/tripbot/internal/database"
	"github.com/logrusorgru/aurora"
)

type Event struct {
	ID          int       `db:"id"`
	Username    string    `db:"username"`
	Event       string    `db:"event"`
	DateCreated time.Time `db:"date_created"`
}

func Login(user string) {
	if config.ReadOnly && config.Verbose {
		log.Printf("Not logging in %s because we're in read-only mode", aurora.Magenta(user))
		return
	}
	tx := database.Connection().MustBegin()
	tx.MustExec("INSERT INTO events (username, event) VALUES ($1, $2)", user, "login")
	tx.Commit()
}

func Logout(user string) {
	if config.ReadOnly && config.Verbose {
		log.Printf("Not logging out %s because we're in read-only mode", aurora.Magenta(user))
		return
	}
	tx := database.Connection().MustBegin()
	tx.MustExec("INSERT INTO events (username, event) VALUES ($1, $2)", user, "logout")
	tx.Commit()
}
