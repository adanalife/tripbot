package events

import (
	"log"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/google/uuid"
	"github.com/logrusorgru/aurora/v3"
)

type Event struct {
	ID          int `gorm:"primaryKey"`
	Username    string
	Event       string
	SessionID   uuid.UUID
	DateCreated time.Time
}

func Login(user string, sessionID uuid.UUID) error {
	if c.Conf.ReadOnly && c.Conf.Verbose {
		log.Printf("Not logging in %s because we're in read-only mode", aurora.Magenta(user))
		return &terrors.ReadOnlyError{Msg: "read-only mode"}
	}
	if err := database.GormDB().Create(&Event{Username: user, Event: "login", SessionID: sessionID}).Error; err != nil {
		return err
	}
	instrumentation.Events.Inc("login")
	return nil
}

func Logout(user string, sessionID uuid.UUID) error {
	if c.Conf.ReadOnly && c.Conf.Verbose {
		log.Printf("Not logging out %s because we're in read-only mode", aurora.Magenta(user))
		return &terrors.ReadOnlyError{Msg: "read-only mode"}
	}
	if err := database.GormDB().Create(&Event{Username: user, Event: "logout", SessionID: sessionID}).Error; err != nil {
		return err
	}
	instrumentation.Events.Inc("logout")
	return nil
}
