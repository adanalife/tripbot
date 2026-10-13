package events

import (
	"context"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	terrors "github.com/adanalife/tripbot/pkg/errors"
)

// ChatLine is one line of chat for RecordChat — a viewer's message or the
// bot's own send, with the clip that was on screen when it was said.
type ChatLine struct {
	Username string
	// UserID and MessageID are the platform's own ids, empty where it reports
	// none (and always for the bot's sends).
	UserID    string
	MessageID string
	Text      string
	// VideoID is the clip airing when the line was said; 0 writes NULL.
	VideoID int
	// TsSec is seconds into that clip; nil writes NULL.
	TsSec *float64
}

// chatRow is the chat_messages table shape. Separate from ChatLine so the
// NULL-able columns are pointers at the storage edge only.
type chatRow struct {
	ID         int64 `gorm:"primaryKey"`
	Platform   string
	Username   string
	UserID     string
	MessageID  string
	Text       string
	VideoID    *int
	VideoTsSec *float64
	EmittedAt  time.Time `gorm:"autoCreateTime"`
}

func (chatRow) TableName() string { return "chat_messages" }

// RecordChat appends one line to the chat log. Like every events writer it
// refuses on a read-only instance and stamps the writing platform itself.
func RecordChat(ctx context.Context, cfg *c.TripbotConfig, l ChatLine) error {
	if cfg.ReadOnly {
		return terrors.ErrReadOnly
	}
	var vid *int
	if l.VideoID != 0 {
		vid = &l.VideoID
	}
	row := chatRow{
		Platform:   cfg.Platform,
		Username:   l.Username,
		UserID:     l.UserID,
		MessageID:  l.MessageID,
		Text:       l.Text,
		VideoID:    vid,
		VideoTsSec: l.TsSec,
	}
	return database.GormDB().WithContext(ctx).Create(&row).Error
}
