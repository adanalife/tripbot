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
	Platform    string
	Event       string
	SessionID   uuid.UUID
	DateCreated time.Time
}

func Login(ctx context.Context, user string, sessionID uuid.UUID) error {
	if c.Conf.ReadOnly && c.Conf.Verbose {
		slog.InfoContext(ctx, "skipping login event: read-only mode", "username", user)
		return &terrors.ReadOnlyError{Msg: "read-only mode"}
	}
	if err := database.GormDB().WithContext(ctx).Create(&Event{Username: user, Platform: c.Conf.Platform, Event: "login", SessionID: sessionID}).Error; err != nil {
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
	if err := database.GormDB().WithContext(ctx).Create(&Event{Username: user, Platform: c.Conf.Platform, Event: "logout", SessionID: sessionID}).Error; err != nil {
		return err
	}
	instrumentation.Events.Inc("logout")
	return nil
}

// SessionCount returns how many sessions the user has started — i.e. their
// count of "login" events. Cheap via the events_username_date index
// (migration 011). Returns 0 on error. Bots are not special-cased here; callers
// that exclude bots should check users.IsBot.
func SessionCount(ctx context.Context, username string) int64 {
	var n int64
	if err := database.GormDB().WithContext(ctx).
		Model(&Event{}).
		Where("platform = ? AND username = ? AND event = ?", c.Conf.Platform, username, "login").
		Count(&n).Error; err != nil {
		slog.ErrorContext(ctx, "session count failed", "err", err, "username", username)
		return 0
	}
	return n
}
