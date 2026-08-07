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
	"sync/atomic"
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
// denormalized so per-clip queries don't need interval pairing; nil before the
// first play of the process or when that play had no DB row.
type ViewerSample struct {
	ID       int `gorm:"primaryKey"`
	Platform string
	Count    int
	VideoID  *int
	// autoCreateTime: see VideoPlay.StartedAt.
	SampledAt time.Time `gorm:"autoCreateTime"`
}

// currentVideoID remembers the most recent play's video id so RecordSample can
// denormalize it without the sessions package knowing the player. 0 means no
// play recorded yet this process.
var currentVideoID atomic.Int64

// RecordPlay writes a video_plays row for a clip switch. Pass videoID 0 when
// the clip has no DB row; the row is written with a NULL video_id.
func RecordPlay(ctx context.Context, cfg *c.TripbotConfig, videoID int, state string, flagged bool, lat, lng float64) {
	currentVideoID.Store(int64(videoID))
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

// RecordSample writes a viewer_samples row for one viewer-count tick, tagged
// with the currently-playing clip as of the last RecordPlay.
func RecordSample(ctx context.Context, cfg *c.TripbotConfig, count int) {
	if cfg.ReadOnly {
		return
	}
	var vid *int
	if id := int(currentVideoID.Load()); id != 0 {
		vid = &id
	}
	sample := ViewerSample{
		Platform: cfg.Platform,
		Count:    count,
		VideoID:  vid,
	}
	if err := database.GormDB().WithContext(ctx).Create(&sample).Error; err != nil {
		slog.ErrorContext(ctx, "error recording viewer sample", "err", err, "count", count)
	}
}
