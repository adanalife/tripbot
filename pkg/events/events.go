package events

import (
	"log"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/logrusorgru/aurora"
)

type Event struct {
	ID          int       `db:"id"`
	Username    string    `db:"username"`
	Event       string    `db:"event"`
	DateCreated time.Time `db:"date_created"`
}

func Login(user string) error {
	if c.Conf.ReadOnly && c.Conf.Verbose {
		log.Printf("Not logging in %s because we're in read-only mode", aurora.Magenta(user))
		return &terrors.ReadOnlyError{Msg: "read-only mode"}
	}
	tx := database.Connection().MustBegin()
	//TODO: do something with result here?
	_, err := tx.Exec("INSERT INTO events (username, event) VALUES ($1, $2)", user, "login")
	if err != nil {
		return err
	}
	return tx.Commit()
}

func Logout(user string) error {
	if c.Conf.ReadOnly && c.Conf.Verbose {
		log.Printf("Not logging out %s because we're in read-only mode", aurora.Magenta(user))
		return &terrors.ReadOnlyError{Msg: "read-only mode"}
	}
	tx := database.Connection().MustBegin()
	//TODO: do something with result here?
	_, err := tx.Exec("INSERT INTO events (username, event) VALUES ($1, $2)", user, "logout")
	if err != nil {
		return err
	}
	return tx.Commit()
}
