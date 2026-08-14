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
	"sync"
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
	// EndedAt is when the clip stopped airing, stamped by the play that
	// supersedes this row. nil means the end was never observed — the process
	// crashed or restarted while the clip was up, or the row predates the
	// column. Duration is derived (EndedAt - StartedAt), never stored.
	EndedAt *time.Time
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

// openPlayIDs remembers, per platform, the id of the video_plays row this
// process opened most recently and hasn't closed, so the next RecordPlay can
// stamp its ended_at. Deliberately process-local rather than looked up from
// the table: a row left open by a previous process stays open forever, because
// a crash or restart means the clip's end was genuinely never observed — a
// NULL ended_at there is honest, and back-dating one would be a guess.
var (
	openPlayMu  sync.Mutex
	openPlayIDs = map[string]int{}
)

// closePreviousPlay stamps ended_at on the platform's still-open play row, if
// this process opened one. Best-effort like the writes around it: a failed
// update logs and leaves the row open.
func closePreviousPlay(ctx context.Context, platform string) {
	openPlayMu.Lock()
	id, ok := openPlayIDs[platform]
	delete(openPlayIDs, platform)
	openPlayMu.Unlock()
	if !ok {
		return
	}
	err := database.GormDB().WithContext(ctx).Model(&VideoPlay{}).
		Where("id = ?", id).Update("ended_at", time.Now()).Error
	if err != nil {
		slog.ErrorContext(ctx, "error closing previous video play", "err", err, "play_id", id)
	}
}

// RecordPlay writes a video_plays row for a clip switch, closing the
// platform's previous row (its clip stopped airing the moment this one
// started). Pass videoID 0 when the clip has no DB row; the row is written
// with a NULL video_id.
func RecordPlay(ctx context.Context, cfg *c.TripbotConfig, videoID int, state string, flagged bool, lat, lng float64) {
	currentVideoID.Store(int64(videoID))
	if cfg.ReadOnly {
		return
	}
	closePreviousPlay(ctx, cfg.Platform)
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
		return
	}
	openPlayMu.Lock()
	openPlayIDs[cfg.Platform] = play.ID
	openPlayMu.Unlock()
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
