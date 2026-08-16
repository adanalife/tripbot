// Package viewstats persists the raw footage-performance signals into
// append-only Postgres tables: one video_plays row per clip switch and one
// viewer_samples row per viewer-count tick. It mirrors the eventbus
// video.changed / viewers.count emissions — which are fire-and-forget over
// NATS — so the history accrues durably. Writes are best-effort: a failed
// insert logs and drops the row rather than disturbing the player/session
// cron ticks that call in.
package viewstats

import (
	"context"
	"log/slog"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
)

// VideoPlay is one video_plays row: a clip landed on screen at StartedAt.
// State/Flagged/Lat/Lng are denormalized at play time because videos rows
// mutate afterwards (coord backfills, state interpolation) — the play row
// records what was true on screen. VideoID is nil when the clip had no DB row
// (a failed LoadOrCreate); the row still marks that a switch happened.
type VideoPlay struct {
	ID       int `gorm:"primaryKey"`
	Platform string
	VideoID  *int
	State    string
	Flagged  bool
	Lat      float64
	Lng      float64
	// autoCreateTime makes GORM stamp the column on insert instead of writing
	// the zero value over its DEFAULT CURRENT_TIMESTAMP. See pkg/events for
	// the full story.
	StartedAt time.Time `gorm:"autoCreateTime"`
}

// ViewerSample is one viewer_samples row: Count chatters were in this
// platform's chat at SampledAt. VideoID is the clip on screen at sample time,
// denormalized so per-clip queries don't need interval pairing; nil when
// nothing was playing or the clip had no DB row.
type ViewerSample struct {
	ID       int `gorm:"primaryKey"`
	Platform string
	// Count is the chatter total — who has spoken, not who is watching. Kept
	// for continuity with the series collected before Viewers existed.
	Count   int
	VideoID *int
	// Viewers is the platform's concurrent-viewer count, and Live whether
	// anything was broadcasting. Both nil when no number was reported, so a
	// rollup can tell "nobody counted" from "nobody watching".
	Viewers *int
	Live    *bool
	// autoCreateTime: see VideoPlay.StartedAt.
	SampledAt time.Time `gorm:"autoCreateTime"`
}

// RecordPlay writes a video_plays row for a clip switch. Pass videoID 0 when
// the clip has no DB row; the row is written with a NULL video_id.
func RecordPlay(ctx context.Context, cfg *c.TripbotConfig, videoID int, state string, flagged bool, lat, lng float64) {
	if cfg.ReadOnly {
		return
	}
	var vid *int
	if videoID != 0 {
		vid = &videoID
	}
	play := VideoPlay{
		Platform: cfg.Platform,
		VideoID:  vid,
		State:    state,
		Flagged:  flagged,
		Lat:      lat,
		Lng:      lng,
	}
	if err := database.GormDB().WithContext(ctx).Create(&play).Error; err != nil {
		slog.ErrorContext(ctx, "error recording video play", "err", err, "video_id", videoID)
	}
}

// Audience is the platform's concurrent-viewer reading for one tick. Reported
// is false when no number was available, which writes NULL rather than a zero
// that would read as "a live broadcast nobody watched".
type Audience struct {
	Count    int
	Live     bool
	Reported bool
}

// RecordSample writes a viewer_samples row for one viewer-count tick.
// chatters is the in-chat total; audience is the watching total, which the two
// columns keep apart because they answer different questions and only one of
// them sizes the audience. videoID is the clip on screen — pass 0 when nothing
// is playing or the clip has no DB row, and the row records a NULL.
//
// The caller reads videoID from the player rather than this package
// remembering the last RecordPlay, so a restart doesn't blind the samples
// taken before the next clip switch.
func RecordSample(ctx context.Context, cfg *c.TripbotConfig, chatters int, audience Audience, videoID int) {
	if cfg.ReadOnly {
		return
	}
	var vid *int
	if videoID != 0 {
		vid = &videoID
	}
	sample := ViewerSample{
		Platform: cfg.Platform,
		Count:    chatters,
		VideoID:  vid,
	}
	if audience.Reported {
		sample.Viewers = &audience.Count
		sample.Live = &audience.Live
	}
	if err := database.GormDB().WithContext(ctx).Create(&sample).Error; err != nil {
		slog.ErrorContext(ctx, "error recording viewer sample", "err", err, "chatters", chatters)
	}
}
