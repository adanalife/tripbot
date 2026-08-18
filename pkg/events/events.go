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
	"github.com/adanalife/tripbot/pkg/helpers"
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
	// rather than []byte because the driver encodes a byte slice as bytea, which
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
	instrumentation.Events.Inc(e.Event, e.Platform)
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

// Follow records that a viewer followed the channel (Twitch channel.follow).
// The durable record behind "followers gained": the EventSub notice otherwise
// only produces a chat shout, so an unwritten follow is unrecoverable. Count
// with DISTINCT username — refollow churn fires the notice again.
func Follow(ctx context.Context, cfg *c.TripbotConfig, user string) error {
	return record(ctx, cfg, Event{Username: user, Event: "follow"})
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

// guessMeta is a guess_submitted event's meta payload. Guessed and Actual are
// post-normalization (two-letter code expanded, close misspelling corrected),
// so rows that differ only in spelling still group together in a rollup.
type guessMeta struct {
	Guessed string `json:"guessed"`
	Actual  string `json:"actual"`
	Correct bool   `json:"correct"`
	// DistanceMi is how far off the guess was: miles between the guessed
	// state's centroid and the actual state's centroid, 0 for a correct
	// guess. Omitted when a centroid doesn't resolve (a territory), since a
	// missing measurement and a perfect one must not read alike.
	DistanceMi *float64 `json:"distance_mi,omitempty"`
}

// guessDistanceMi measures a guess's miss distance in centroid-to-centroid
// miles. A correct guess is definitionally 0 — even for a territory with no
// centroid to measure from. nil when either centroid doesn't resolve. Pure
// arithmetic over the built-in centroid table; no I/O.
func guessDistanceMi(g GuessSubmission) *float64 {
	if g.Correct {
		zero := 0.0
		return &zero
	}
	glat, glng, ok := helpers.StateCentroid(g.Guessed)
	if !ok {
		return nil
	}
	alat, alng, ok := helpers.StateCentroid(g.Actual)
	if !ok {
		return nil
	}
	d := helpers.MilesBetween(glat, glng, alat, alng)
	return &d
}

// GuessSubmission describes one answerable !guess for GuessSubmitted.
type GuessSubmission struct {
	Username string
	// Guessed is the state the viewer guessed, normalized.
	Guessed string
	// Actual is the state the aired footage is in.
	Actual string
	// Correct is whether Guessed matched Actual.
	Correct bool
	// VideoID is the clip being guessed at; 0 writes NULL.
	VideoID int
	// TsSec is seconds into that clip; nil writes NULL.
	TsSec *float64
}

// GuessSubmitted records one answerable !guess — right or wrong — pairing what
// the viewer guessed with what was actually on screen, plus how far off it was
// in centroid miles. Guesses at footage with no known state are deliberately
// not recorded: there is no right answer to compare against, so a row would
// only skew any accuracy computed from the log. The distance is derived here,
// at the single write point, so no emit site can forget it.
func GuessSubmitted(ctx context.Context, cfg *c.TripbotConfig, g GuessSubmission) error {
	payload, err := json.Marshal(guessMeta{
		Guessed:    g.Guessed,
		Actual:     g.Actual,
		Correct:    g.Correct,
		DistanceMi: guessDistanceMi(g),
	})
	if err != nil {
		return err
	}
	meta := string(payload)
	var vid *int
	if g.VideoID != 0 {
		vid = &g.VideoID
	}
	return record(ctx, cfg, Event{
		Username:   g.Username,
		Event:      "guess_submitted",
		Meta:       &meta,
		VideoID:    vid,
		VideoTsSec: g.TsSec,
	})
}

// Warp sources carried in a timewarp event's meta. Every path that jumps the
// playhead to a random clip names itself here, so per-source warp counts can't
// silently lump triggers together.
const (
	// WarpSourceCommand — a viewer ran !timewarp.
	WarpSourceCommand = "command"
	// WarpSourceGuess — a correct !guess triggered the warp as its reward.
	WarpSourceGuess = "guess"
	// WarpSourceGift — a platform gift's effect triggered the warp.
	WarpSourceGift = "gift"
)

// timewarpMeta is a timewarp event's meta payload: how the warp was triggered
// and where it landed. The row's video_id carries the clip the warp left.
type timewarpMeta struct {
	Source string `json:"source"`
	// To is the videos.id of the clip the warp landed on, omitted when the
	// new clip has no DB row.
	To int `json:"to,omitempty"`
}

// Warp describes one playhead warp for Timewarp.
type Warp struct {
	// Username is the viewer whose action triggered the warp.
	Username string
	// Source names the trigger (the WarpSource* constants).
	Source string
	// VideoID is the clip airing before the warp; 0 writes NULL.
	VideoID int
	// TsSec is seconds into that clip when the warp fired; nil writes NULL.
	TsSec *float64
	// ToVideoID is the clip the warp landed on; 0 omits the meta key.
	ToVideoID int
}

// Timewarp records a playhead warp to a random clip: who triggered it, how,
// and the from/to clips. Paired with video_plays this answers where warps pull
// viewers from — which footage people warp away from, and what they land on.
func Timewarp(ctx context.Context, cfg *c.TripbotConfig, w Warp) error {
	payload, err := json.Marshal(timewarpMeta{Source: w.Source, To: w.ToVideoID})
	if err != nil {
		return err
	}
	meta := string(payload)
	var vid *int
	if w.VideoID != 0 {
		vid = &w.VideoID
	}
	return record(ctx, cfg, Event{
		Username:   w.Username,
		Event:      "timewarp",
		Meta:       &meta,
		VideoID:    vid,
		VideoTsSec: w.TsSec,
	})
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

// commandMeta is the meta payload shared by command_run and command_refused
// events: the same command/typed/args fields describe both outcomes, and only
// a refusal carries a reason.
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

// CommandRun describes one dispatched command for CommandRan.
type CommandRun struct {
	Username string
	// Command is the canonical trigger (`!location`).
	Command string
	// Typed is the token the viewer actually wrote when it differs from
	// Command — the alias, state shortcut, or misspelling they reached for.
	Typed string
	// Args is the remainder of the message, kept because a command's arguments
	// are often the point (which state they guessed, what they searched for).
	Args string
	// VideoID is the clip airing when the command ran; 0 writes NULL.
	VideoID int
	// TsSec is seconds into that clip; nil writes NULL.
	TsSec *float64
}

// CommandRan records a command the bot dispatched and ran. Paired with
// command_refused it makes the command surface fully accountable: every
// attempt lands in exactly one of the two kinds, so usage, distinct users,
// and refusal rates are all queries over the log.
func CommandRan(ctx context.Context, cfg *c.TripbotConfig, r CommandRun) error {
	payload, err := json.Marshal(commandMeta{
		Command: r.Command,
		Typed:   r.Typed,
		Args:    r.Args,
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
		Event:      "command_run",
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

// deployMeta is a deploy event's meta payload.
type deployMeta struct {
	Component string `json:"component"`
	Version   string `json:"version"`
}

// lastDeployVersion returns the version carried by the platform's most recent
// deploy event for component, or "" when it has none.
func lastDeployVersion(ctx context.Context, platform, component string) (string, error) {
	var last sql.NullString
	err := database.GormDB().WithContext(ctx).
		Model(&Event{}).
		Where("platform = ? AND event = ? AND meta->>'component' = ?", platform, "deploy", component).
		Order("date_created DESC").
		Limit(1).
		Select("meta->>'version'").
		Scan(&last).Error
	if err != nil {
		return "", err
	}
	return last.String, nil
}

// Deploy records that component is now running version — a system event (no
// actor), written at startup. A version already carried by the platform's most
// recent deploy event for that component is skipped, so a pod restart on the
// same build doesn't re-record a deploy that already happened; only a version
// change is a deploy. Returns whether a row was written.
//
// The dedup read is best-effort: when it fails, the row is recorded anyway. A
// duplicate deploy row is a nuisance; a missing one is a hole in the ops
// timeline that nothing else remembers once metrics retention expires.
func Deploy(ctx context.Context, cfg *c.TripbotConfig, component, version string) (bool, error) {
	if cfg.ReadOnly {
		return false, terrors.ErrReadOnly
	}
	last, err := lastDeployVersion(ctx, cfg.Platform, component)
	if err != nil {
		slog.WarnContext(ctx, "deploy dedup read failed; recording anyway", "err", err)
	} else if last == version {
		return false, nil
	}
	payload, err := json.Marshal(deployMeta{Component: component, Version: version})
	if err != nil {
		return false, err
	}
	meta := string(payload)
	if err := record(ctx, cfg, Event{Event: "deploy", Meta: &meta}); err != nil {
		return false, err
	}
	return true, nil
}

// Watchdog restart outcomes carried in a watchdog_restart event's meta.
const (
	// WatchdogOutcomeOK — the recovery action completed (the OBS output
	// restarted / the room re-minted). Whether the recovery *held* is what a
	// later watchdog_recovered event records.
	WatchdogOutcomeOK = "ok"
	// WatchdogOutcomeFailed — the recovery action itself errored.
	WatchdogOutcomeFailed = "failed"
)

// watchdogMeta is a watchdog_restart / watchdog_recovered event's meta payload.
type watchdogMeta struct {
	// Watchdog names which watchdog fired — the platform whose stream it
	// guards.
	Watchdog string `json:"watchdog"`
	// Outcome is WatchdogOutcomeOK / WatchdogOutcomeFailed on a
	// watchdog_restart; a watchdog_recovered carries none.
	Outcome string `json:"outcome,omitempty"`
}

// WatchdogRestart records the stream watchdog forcing a recovery: a silent
// disconnect was declared and the restart action ran, with the given outcome.
// A system event — the ops transition that otherwise only exists in logs,
// which is how the 2026-08-05 outage timeline had to be reconstructed.
func WatchdogRestart(ctx context.Context, cfg *c.TripbotConfig, watchdog, outcome string) error {
	payload, err := json.Marshal(watchdogMeta{Watchdog: watchdog, Outcome: outcome})
	if err != nil {
		return err
	}
	meta := string(payload)
	return record(ctx, cfg, Event{Event: "watchdog_restart", Meta: &meta})
}

// WatchdogRecovered records a watchdog-forced recovery observed to hold: the
// channel stayed live long enough that the watchdog retired its restart
// cooldown. Paired with the watchdog_restart rows before it, the gap is the
// outage's queryable duration.
func WatchdogRecovered(ctx context.Context, cfg *c.TripbotConfig, watchdog string) error {
	payload, err := json.Marshal(watchdogMeta{Watchdog: watchdog})
	if err != nil {
		return err
	}
	meta := string(payload)
	return record(ctx, cfg, Event{Event: "watchdog_recovered", Meta: &meta})
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

// raidMeta is a raid event's meta payload.
type raidMeta struct {
	// From is the raiding channel's name, duplicated from the row's username
	// so the payload stays self-contained when queried by kind.
	From string `json:"from"`
	// Viewers is the raiding party size Twitch reported.
	Viewers int `json:"viewers"`
}

// Raid describes one incoming raid for Raided.
type Raid struct {
	// From is the raiding channel's name.
	From string
	// Viewers is how many viewers arrived with the raid.
	Viewers int
	// VideoID is the clip airing when the raid landed; 0 writes NULL.
	VideoID int
	// TsSec is seconds into that clip; nil writes NULL.
	TsSec *float64
}

// Raided records an incoming raid, stamped with the clip it landed on. A raid
// dumps its viewers onto whatever footage happens to be airing, which inflates
// any per-clip audience metric computed from the log — recording the raid is
// what lets a rollup control for the spike instead of crediting the footage.
func Raided(ctx context.Context, cfg *c.TripbotConfig, r Raid) error {
	payload, err := json.Marshal(raidMeta{From: r.From, Viewers: r.Viewers})
	if err != nil {
		return err
	}
	meta := string(payload)
	var vid *int
	if r.VideoID != 0 {
		vid = &r.VideoID
	}
	return record(ctx, cfg, Event{
		Username:   r.From,
		Event:      "raid",
		Meta:       &meta,
		VideoID:    vid,
		VideoTsSec: r.TsSec,
	})
}

// consoleActionMeta is a console_action event's meta payload.
type consoleActionMeta struct {
	Action string `json:"action"`
	Target string `json:"target"`
	Detail string `json:"detail,omitempty"`
}

// ConsoleAction records one successful admin mutation performed through the
// standalone console — the audit trail for "what changed from the console,
// and when". A system event (empty username): the console holds no
// per-operator identity to attribute. No airing context — console actions are
// about the platform's controls, not the footage on screen.
func ConsoleAction(ctx context.Context, cfg *c.TripbotConfig, action, target, detail string) error {
	payload, err := json.Marshal(consoleActionMeta{Action: action, Target: target, Detail: detail})
	if err != nil {
		return err
	}
	meta := string(payload)
	return record(ctx, cfg, Event{Event: "console_action", Meta: &meta})
}
