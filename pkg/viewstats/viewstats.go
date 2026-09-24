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
	"github.com/adanalife/tripbot/pkg/events"
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
	// ChatMessages is how many chat messages arrived in the sampling window
	// ending at this row. nil means no counter was wired that tick; 0 means
	// wired and silent. Commands and bots' messages count — the total is an
	// aggregate no reader can attribute to senders.
	ChatMessages *int
	// autoCreateTime: see VideoPlay.StartedAt.
	SampledAt time.Time `gorm:"autoCreateTime"`
}

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

// Audience is the platform's concurrent-viewer reading for one tick. Reported
// is false when no number was available, which writes NULL rather than a zero
// that would read as "a live broadcast nobody watched".
type Audience struct {
	Count    int
	Live     bool
	Reported bool
}

// peaks caches, per platform, the highest concurrent-viewer count the series
// holds, so the record check costs a comparison rather than a query every tick.
// Seeded once per platform from the table itself, which is what makes it
// survive a restart: the high is a property of the history, not of this
// process. Unlike openPlayIDs above, a value carried over from a previous
// process is exactly what's wanted here.
var (
	peakMu sync.Mutex
	peaks  = map[string]int{}
)

// storedPeak reads the platform's highest recorded concurrent-viewer count.
// 0 when the series holds none — an empty table, or a platform that has never
// had a number reported. Errors read as 0 and log, which costs at most one
// missed record announcement rather than a wrong one, because the very next
// tick reseeds.
func storedPeak(ctx context.Context, platform string) int {
	var peak int
	err := database.GormDB().WithContext(ctx).Model(&ViewerSample{}).
		Where("platform = ?", platform).
		Select("COALESCE(MAX(viewers), 0)").Scan(&peak).Error
	if err != nil {
		slog.ErrorContext(ctx, "error reading peak viewer count", "err", err, "platform", platform)
		return 0
	}
	return peak
}

// checkPeak emits a viewer_record event when this tick's audience beats the
// platform's all-time high. Called before the sample row is written, so the
// high it compares against is the history this reading is about to join rather
// than one that already includes it.
//
// An unreported or zero count can't set a record: NULL means nobody counted,
// and nobody watching is not a milestone.
func checkPeak(ctx context.Context, cfg *c.TripbotConfig, audience Audience) {
	if !audience.Reported || audience.Count <= 0 {
		return
	}

	peakMu.Lock()
	previous, seeded := peaks[cfg.Platform]
	if !seeded {
		previous = storedPeak(ctx, cfg.Platform)
	}
	beaten := audience.Count > previous
	if beaten || !seeded {
		peaks[cfg.Platform] = max(previous, audience.Count)
	}
	peakMu.Unlock()

	if !beaten {
		return
	}
	if previous == 0 {
		// Nothing to beat. Announcing "a new all-time high" against an empty
		// series would make the first number ever reported a milestone, so the
		// bar gets set silently and the next reading that clears it is the
		// first real record.
		return
	}
	if err := events.ViewerRecord(ctx, cfg, audience.Count, previous); err != nil {
		slog.ErrorContext(ctx, "error recording viewer record", "err", err,
			"viewers", audience.Count, "previous", previous)
	}
}

// RecordSample writes a viewer_samples row for one viewer-count tick.
// chatters is the in-chat total; audience is the watching total, which the two
// columns keep apart because they answer different questions and only one of
// them sizes the audience. videoID is the clip on screen — pass 0 when nothing
// is playing or the clip has no DB row, and the row records a NULL.
// chatMessages is the tick's chat-message tally; nil (no counter wired) records
// NULL, which is distinct from a wired-and-silent 0.
//
// The caller reads videoID from the player rather than this package
// remembering the last RecordPlay, so a restart doesn't blind the samples
// taken before the next clip switch.
func RecordSample(ctx context.Context, cfg *c.TripbotConfig, chatters int, audience Audience, videoID int, chatMessages *int) {
	if cfg.ReadOnly {
		return
	}
	checkPeak(ctx, cfg, audience)
	var vid *int
	if videoID != 0 {
		vid = &videoID
	}
	sample := ViewerSample{
		Platform:     cfg.Platform,
		Count:        chatters,
		VideoID:      vid,
		ChatMessages: chatMessages,
	}
	if audience.Reported {
		sample.Viewers = &audience.Count
		sample.Live = &audience.Live
	}
	if err := database.GormDB().WithContext(ctx).Create(&sample).Error; err != nil {
		slog.ErrorContext(ctx, "error recording viewer sample", "err", err, "chatters", chatters)
	}
}

// MessageCounter tallies inbound chat messages between sample ticks: the chat
// handler increments it once per message and the session tick drains it, so
// each viewer_samples row carries the messages that arrived in its window.
// cmd/tripbot constructs one and hands it to both sides (it satisfies both
// chatbot.ChatCounter and users.ChatCounter). Safe for concurrent use.
type MessageCounter struct{ n atomic.Int64 }

// Add counts one inbound chat message.
func (m *MessageCounter) Add() { m.n.Add(1) }

// Drain returns the count accumulated since the last Drain and resets it.
func (m *MessageCounter) Drain() int { return int(m.n.Swap(0)) }
