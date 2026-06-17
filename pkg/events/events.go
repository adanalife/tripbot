package events

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/google/uuid"
)

type Event struct {
	ID        int `gorm:"primaryKey"`
	Username  string
	Platform  string
	Event     string
	SessionID uuid.UUID
	// autoCreateTime makes GORM stamp date_created with the current time on
	// insert. Without it, GORM writes the zero value (0001-01-01) into the
	// column — overriding its DEFAULT CURRENT_TIMESTAMP — which froze every
	// event written after the GORM migration (#499) at year 1.
	DateCreated time.Time `gorm:"autoCreateTime"`
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

// preFixSentinel is safely after the 0001-01-01 zero-time the timestamp bug
// wrote (between the GORM migration #499 and the autoCreateTime fix) but well
// before any real stream data — the stream started May 2019. Used to exclude
// the bogus zero-dated rows when reconstructing a user's first-seen date.
var preFixSentinel = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// EarliestRealEventDate returns the earliest event timestamp for the user that
// isn't the 0001-01-01 sentinel left by the date_created bug, i.e. the best
// available evidence of when we first saw them. Returns the zero time if the
// user has no real-dated events (all their events fell in the bug window, or
// they have none). Cheap via the events_username_date index (migration 011).
func EarliestRealEventDate(ctx context.Context, platform, username string) time.Time {
	var earliest sql.NullTime
	if err := database.GormDB().WithContext(ctx).
		Model(&Event{}).
		Where("platform = ? AND username = ? AND date_created > ?", platform, username, preFixSentinel).
		Select("MIN(date_created)").
		Scan(&earliest).Error; err != nil {
		slog.ErrorContext(ctx, "earliest event date failed", "err", err, "username", username)
		return time.Time{}
	}
	if !earliest.Valid {
		return time.Time{}
	}
	return earliest.Time
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
