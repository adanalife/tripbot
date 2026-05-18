package events

import (
	"context"
	"log/slog"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/google/uuid"
)

type Event struct {
	ID          int `gorm:"primaryKey"`
	Username    string
	Event       string
	SessionID   uuid.UUID
	DateCreated time.Time
}

func Login(ctx context.Context, user string, sessionID uuid.UUID) error {
	if c.Conf.ReadOnly && c.Conf.Verbose {
		slog.InfoContext(ctx, "skipping login event: read-only mode", "username", user)
		return &terrors.ReadOnlyError{Msg: "read-only mode"}
	}
	if err := database.GormDB().WithContext(ctx).Create(&Event{Username: user, Event: "login", SessionID: sessionID}).Error; err != nil {
		return err
	}
	instrumentation.Events.Inc("login")
	return nil
}

func Logout(ctx context.Context, user string, sessionID uuid.UUID) error {
	if c.Conf.ReadOnly && c.Conf.Verbose {
		slog.InfoContext(ctx, "skipping logout event: read-only mode", "username", user)
		return &terrors.ReadOnlyError{Msg: "read-only mode"}
	}
	if err := database.GormDB().WithContext(ctx).Create(&Event{Username: user, Event: "logout", SessionID: sessionID}).Error; err != nil {
		return err
	}
	instrumentation.Events.Inc("logout")
	return nil
}
