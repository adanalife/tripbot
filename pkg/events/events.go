package events

import (
	"log"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/google/uuid"
	"github.com/logrusorgru/aurora/v3"
)

type Event struct {
	ID          int       `db:"id"`
	Username    string    `db:"username"`
	Event       string    `db:"event"`
	SessionID   uuid.UUID `db:"session_id"`
	DateCreated time.Time `db:"date_created"`
}

func Login(user string, sessionID uuid.UUID) error {
	if c.Conf.ReadOnly && c.Conf.Verbose {
		log.Printf("Not logging in %s because we're in read-only mode", aurora.Magenta(user))
		return &terrors.ReadOnlyError{Msg: "read-only mode"}
	}
	tx := database.Connection().MustBegin()
	_, err := tx.Exec("INSERT INTO events (username, event, session_id) VALUES ($1, $2, $3)", user, "login", sessionID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func Logout(user string, sessionID uuid.UUID) error {
	if c.Conf.ReadOnly && c.Conf.Verbose {
		log.Printf("Not logging out %s because we're in read-only mode", aurora.Magenta(user))
		return &terrors.ReadOnlyError{Msg: "read-only mode"}
	}
	tx := database.Connection().MustBegin()
	_, err := tx.Exec("INSERT INTO events (username, event, session_id) VALUES ($1, $2, $3)", user, "logout", sessionID)
	if err != nil {
		return err
	}
	return tx.Commit()
}
