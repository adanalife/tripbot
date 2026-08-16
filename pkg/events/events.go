package events

import (
	"context"
	"database/sql"
	"encoding/json"
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
	// ExtraMilesEarned records, on a logout event, the bonus portion of the
	// session the events pairing can't reconstruct — community sub-grants
	// received plus the 5% subscriber bonus. Pointer so it writes NULL on
	// every non-logout event and on zero-extra logouts; SUM treats NULL and 0
	// identically, so a future rollup can add it to the events-derived base.
	ExtraMilesEarned *float64 `gorm:"column:extra_miles_earned"`
	// VideoID and VideoTsSec record what was airing when the event happened:
	// the clip's videos.id and, for writers that know the playhead, seconds
	// into the aired clip. Pointers so kinds with no airing context write
	// NULL.
	VideoID    *int     `gorm:"column:video_id"`
	VideoTsSec *float64 `gorm:"column:video_ts_sec"`
	// Meta is the kind-specific payload, one JSONB document per row. A string
	// rather than []byte because lib/pq encodes a byte slice as bytea, which
	// Postgres refuses to coerce into jsonb (see pkg/rotatorstore).
	Meta *string `gorm:"type:jsonb"`
}

// Airing is what was on screen when an event happened. The zero value carries
// no airing context and writes NULL into both columns — what an instance with
// no player records, and what a viewer joining an idle stream gets.
type Airing struct {
	// VideoID is the clip's videos.id. 0 writes NULL, covering both "nothing
	// playing" and "the clip has no DB row".
	VideoID int
	// TsSec is seconds into that clip. nil writes NULL, for a writer that
	// knows the clip but not the playhead.
	TsSec *float64
}

// apply stamps the airing columns onto an event being built.
func (a Airing) apply(e *Event) {
	if a.VideoID != 0 {
		vid := a.VideoID
		e.VideoID = &vid
	}
	e.VideoTsSec = a.TsSec
}

// record writes one event row and counts it. Every event kind goes through
// here, so the read-only guard, the platform stamp and the metric can't be
// forgotten by a new kind. The caller supplies only the fields its kind uses;
// the rest write as NULL / zero.
func record(ctx context.Context, cfg *c.TripbotConfig, e Event) error {
	if cfg.ReadOnly {
		return terrors.ErrReadOnly
	}
	e.Platform = cfg.Platform
	if err := database.GormDB().WithContext(ctx).Create(&e).Error; err != nil {
		return err
	}
	instrumentation.Events.Inc(e.Event)
	return nil
}

// Login records a session-start event. airing is the footage the viewer
// arrived on, which is what pairs a join against the clip that earned it.
func Login(ctx context.Context, cfg *c.TripbotConfig, user string, sessionID uuid.UUID, airing Airing) error {
	e := Event{Username: user, Event: "login", SessionID: sessionID}
	airing.apply(&e)
	return record(ctx, cfg, e)
}

// Logout records a session-end event. extraMiles is the session's
// unreconstructable bonus (sub-grants + 5% bonus); pass nil to write NULL
// when it's zero. airing is the footage the viewer left on — the other half
// of the per-clip join/leave churn the login row opens.
func Logout(ctx context.Context, cfg *c.TripbotConfig, user string, sessionID uuid.UUID, extraMiles *float64, airing Airing) error {
	e := Event{Username: user, Event: "logout", SessionID: sessionID, ExtraMilesEarned: extraMiles}
	airing.apply(&e)
	return record(ctx, cfg, e)
}

// Subscribe records that a viewer's subscription began (Twitch
// channel.subscribe — initial subs and gift-sub recipients). Paired with
// Unsubscribe it bounds a viewer's subscribed interval, which is what the 5%
// miles bonus keys off. No session_id: this isn't a login/logout.
func Subscribe(ctx context.Context, cfg *c.TripbotConfig, user string) error {
	return record(ctx, cfg, Event{Username: user, Event: "subscribe"})
}

// Unsubscribe records that a viewer's subscription ended (Twitch
// channel.subscription.end — real lapse/cancel, never a guessed expiry).
// Closes the interval Subscribe opened.
func Unsubscribe(ctx context.Context, cfg *c.TripbotConfig, user string) error {
	return record(ctx, cfg, Event{Username: user, Event: "unsubscribe"})
}

// Correction records a manual miles adjustment (delta, may be negative) as an
// event carrying the amount in extra_miles_earned, so the rollup folds it into
// user_rollups.extra_miles alongside the session bonuses. This is the audit
// trail for out-of-band miles changes the login/logout pairing can't see.
func Correction(ctx context.Context, cfg *c.TripbotConfig, user string, delta float64) error {
	return record(ctx, cfg, Event{Username: user, Event: "correction", ExtraMilesEarned: &delta})
}

// stateCrossingMeta is a state_crossing event's meta payload.
type stateCrossingMeta struct {
	From string `json:"from"`
	To   string `json:"to"`
	// Sequential is true when the new clip follows the previous one in corpus
	// order (next_vid) — the van genuinely drove across the line — as opposed
	// to a timewarp/skip landing the playhead in another state.
	Sequential bool `json:"sequential"`
}

// StateCrossing records the aired footage entering a new US state. A system
// event — no viewer did it — so Username is empty. Granularity is clip-level:
// it fires when a clip switch changes the state, not at the exact frame the
// line is crossed. Pass videoID 0 when the new clip has no DB row.
func StateCrossing(ctx context.Context, cfg *c.TripbotConfig, from, to string, videoID int, sequential bool) error {
	payload, err := json.Marshal(stateCrossingMeta{From: from, To: to, Sequential: sequential})
	if err != nil {
		return err
	}
	meta := string(payload)
	var vid *int
	if videoID != 0 {
		vid = &videoID
	}
	return record(ctx, cfg, Event{Event: "state_crossing", Meta: &meta, VideoID: vid})
}

// Refusal reasons carried in a command_refused event's meta. Every path that
// declines to run a command names itself here, so a refusal rate computed from
// the log can't silently omit one.
const (
	// RefusedUnknown — the token matches no command anywhere in the registry:
	// a typo, or a trigger from another channel's bot.
	RefusedUnknown = "unknown"
	// RefusedWrongPlatform — the command exists but isn't indexed for this
	// platform, so it was unreachable rather than mistyped.
	RefusedWrongPlatform = "wrong_platform"
	// RefusedFollowGate — the command requires following and the viewer isn't.
	RefusedFollowGate = "follow_gate"
	// RefusedSubGate — the command requires a subscription and the viewer
	// isn't subscribed. Only reachable where the platform reports subscribers.
	RefusedSubGate = "sub_gate"
	// RefusedCooldown — the viewer ran it too recently to run it again.
	RefusedCooldown = "cooldown"
)

// commandMeta is a command_refused event's meta payload. Shaped for command_run
// to share once it ships: that kind wants the same command/typed/args fields
// and no reason.
type commandMeta struct {
	// Command is the canonical trigger (`!location`), or the raw token when
	// nothing matched and there is no canonical form to report.
	Command string `json:"command"`
	// Typed is the token as the viewer actually wrote it, recorded only when it
	// differs from Command — the alias or casing they reached for, which is
	// what tells you an alias is worth promoting.
	Typed string `json:"typed,omitempty"`
	// Args is the remainder of the message, kept because a refusal's arguments
	// are often the point (which state they guessed, what they searched for).
	Args   string `json:"args,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// CommandRefusal describes one declined command for CommandRefused.
type CommandRefusal struct {
	Username string
	Command  string
	Typed    string
	Args     string
	Reason   string
	// VideoID is the clip airing when the refusal happened; 0 writes NULL.
	VideoID int
	// TsSec is seconds into that clip; nil writes NULL.
	TsSec *float64
}

// CommandRefused records a command the bot declined to run, with the reason.
// This is the queryable replacement for reading refusals out of the logs —
// "which commands do people reach for that don't exist" is the question that
// seeds new commands, and it's unrecoverable if it isn't written down.
func CommandRefused(ctx context.Context, cfg *c.TripbotConfig, r CommandRefusal) error {
	payload, err := json.Marshal(commandMeta{
		Command: r.Command,
		Typed:   r.Typed,
		Args:    r.Args,
		Reason:  r.Reason,
	})
	if err != nil {
		return err
	}
	meta := string(payload)
	var vid *int
	if r.VideoID != 0 {
		vid = &r.VideoID
	}
	return record(ctx, cfg, Event{
		Username:   r.Username,
		Event:      "command_refused",
		Meta:       &meta,
		VideoID:    vid,
		VideoTsSec: r.TsSec,
	})
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
func SessionCount(ctx context.Context, platform, username string) int64 {
	var n int64
	if err := database.GormDB().WithContext(ctx).
		Model(&Event{}).
		Where("platform = ? AND username = ? AND event = ?", platform, username, "login").
		Count(&n).Error; err != nil {
		slog.ErrorContext(ctx, "session count failed", "err", err, "username", username)
		return 0
	}
	return n
}
