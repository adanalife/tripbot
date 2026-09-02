package video

import (
	"context"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"log/slog"
	"sync"
	"time"

	"github.com/adanalife/tripbot/pkg/eventbus"
	"github.com/adanalife/tripbot/pkg/events"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	playoutClient "github.com/adanalife/tripbot/pkg/playout-client"
	"github.com/adanalife/tripbot/pkg/viewstats"
)

// onscreens is the subset of the onscreens-client surface the Player drives
// (GPS overlay toggles on flagged-video transitions). Tests inject a
// recording fake; production uses *onscreensClient.Client, which mirrors
// each call to NATS + HTTP.
type onscreens interface {
	ShowGPSImage(ctx context.Context, dur time.Duration) error
	HideGPSImage(ctx context.Context) error
}

// Player owns the state of "what's currently playing" and the clients that
// drive the Playout playback + onscreens overlays. Construct via NewPlayer; the
// single process-wide instance lives on cmd/tripbot's Tripbot struct.
type Player struct {
	CurrentlyPlaying Video // exported because external callers read the current video off it
	curVid, preVid   string
	timeStarted      time.Time
	cfg              *c.TripbotConfig
	onscreens        onscreens
	playout          *playoutClient.Client

	// airing is the US state the footage on screen was last seen in, and the
	// clip it was seen in — what TrackState compares each tick against. Its own
	// lock because TrackState runs on a faster cron than GetCurrentlyPlaying.
	airingMu sync.Mutex
	airing   airing
}

// airing is one observation of where the footage on screen is.
type airing struct {
	state string
	vid   Video
}

// NewPlayer returns a Player with its own Onscreens + Playout clients. cfg
// supplies the env + platform its video.changed events are tagged with and
// the read-only gate its video_plays writes honor.
func NewPlayer(cfg *c.TripbotConfig, onscreens onscreens, playout *playoutClient.Client) *Player {
	return &Player{cfg: cfg, onscreens: onscreens, playout: playout}
}

// GetCurrentlyPlaying asks Playout which dashcam video is on screen and
// advances the Player's state if it changed.
// ctx carries the cron tick's trace span; it isn't propagated into the
// playout-client / onscreens-client HTTP calls (those clients don't take a ctx,
// so the underlying Playout poll and GPS-image toggles don't nest as children).
// TODO: consider making this return a video struct
func (p *Player) GetCurrentlyPlaying(ctx context.Context) {
	var err error

	// save the video we used last time
	p.preVid = p.curVid

	// figure out what's currently playing
	p.curVid = p.playout.CurrentlyPlaying(ctx)

	// if the currently-playing video has changed
	if p.curVid != p.preVid {
		// reset the stopwatch
		p.timeStarted = time.Now()

		// share the Video with the system
		p.CurrentlyPlaying, err = LoadOrCreate(ctx, p.curVid)
		if err != nil {
			// Downstream of playout.CurrentlyPlaying; the wrapper there already
			// logged the root cause at Error. Debug-level keeps the breadcrumb
			// without double-counting in Sentry.
			slog.DebugContext(ctx, "unable to create Video", "err", err, "file", p.curVid)
		}

		slog.InfoContext(ctx, "now playing",
			"file", p.CurrentlyPlaying.File(),
			"state", helpers.StateToStateAbbrev(p.CurrentlyPlaying.State),
		)

		// Announce the switch so the admin panel's "now playing" card updates
		// live (no-op when NATS is unconfigured). emitted_at doubles as the
		// clip start time for the panel's elapsed ticker.
		eventbus.EmitVideoChanged(ctx, p.cfg.Environment, p.cfg.Platform,
			p.CurrentlyPlaying.File(), p.CurrentlyPlaying.State, p.CurrentlyPlaying.Flagged,
			p.CurrentlyPlaying.Lat, p.CurrentlyPlaying.Lng)

		// Persist the switch as a video_plays row — the durable half of the
		// emission above (NATS core is fire-and-forget). The first tick after a
		// restart records a fresh play for the clip already on screen, since its
		// true start time wasn't observed.
		viewstats.RecordPlay(ctx, p.cfg, p.CurrentlyPlaying.ID, p.CurrentlyPlaying.State,
			p.CurrentlyPlaying.Flagged, p.CurrentlyPlaying.Lat, p.CurrentlyPlaying.Lng)

		// show the no-GPS image
		if p.CurrentlyPlaying.Flagged {
			// the duration is ignored — the server owns the GPS overlay's duration
			p.onscreens.ShowGPSImage(ctx, 60*time.Second)
		} else {
			p.onscreens.HideGPSImage(ctx)
		}
	}
}

// TrackState records a state_crossing event when the footage on screen has
// entered a new US state, and keeps the current-state gauge on that state.
//
// It reads the playhead rather than the clip: video_coords knows which state
// each moment of a clip is in, so a line crossed mid-clip is recorded at the
// moment it happens — 48 clips in the corpus cross one — instead of at the
// next clip switch. A clip with no track answers with its clip-level state,
// which is what the switch-time comparison used to do; the first observation
// after boot records nothing, since there is nothing to cross from.
//
// sequential says the van drove across the line: either the same clip is still
// on screen, or the new clip follows the previous one in corpus order
// (next_vid). Anything else is a playhead jump landing in another state.
// ponytail: a !seek within one clip also reads as sequential; the seek is a
// few seconds of the same road, so the answer is right often enough.
//
// Best-effort, like the video_plays write: a failed insert logs and drops the
// event rather than disturbing the tick.
func (p *Player) TrackState(ctx context.Context) {
	vid, at := p.Playhead(ctx)
	if vid.Slug == "" {
		return
	}
	state, moment := vid.State, events.Airing{VideoID: vid.ID}
	if m, ok := CoordAt(ctx, vid, at); ok && m.State != "" {
		sec := at.Seconds()
		state, moment.TsSec = m.State, &sec
	}
	if state == "" {
		return
	}

	p.airingMu.Lock()
	prev := p.airing
	p.airing = airing{state: state, vid: vid}
	p.airingMu.Unlock()

	// A blank abbrev (unresolvable state) records as "unknown" so a stuck
	// playhead is alertable.
	instrumentation.CurrentState.Set(helpers.StateToStateAbbrev(state), p.cfg.Platform)

	if prev.state == "" || prev.state == state {
		return
	}
	sequential := prev.vid.ID == vid.ID ||
		(prev.vid.NextVid.Valid && int(prev.vid.NextVid.Int64) == vid.ID)
	if err := events.StateCrossing(ctx, p.cfg, prev.state, state, moment, sequential); err != nil {
		slog.ErrorContext(ctx, "error recording state crossing", "err", err)
	}
}

// CurrentProgress represents how long the video has been playing
// it will be useful eventually for choosing the exact right screenshot
func (p *Player) CurrentProgress() time.Duration {
	return time.Since(p.timeStarted)
}

// Playhead returns the clip on screen and how far into it the stream is.
//
// The stopwatch behind CurrentProgress starts when the cron *notices* a clip
// change, and it only looks once a minute, so both the clip and the offset can
// be that far behind — a kilometre and a half of driving at highway speed,
// which is more than enough to undo the precision a per-moment coordinate
// buys. playout reports its real file and position every five seconds, so that
// is the answer whenever it's readable; the Player's own state is the fallback
// for when it isn't.
func (p *Player) Playhead(ctx context.Context) (Video, time.Duration) {
	file, pos, ok := p.playout.Playhead(ctx)
	if !ok {
		return p.CurrentlyPlaying, p.CurrentProgress()
	}
	if slug(file) == p.CurrentlyPlaying.Slug {
		return p.CurrentlyPlaying, pos
	}
	// playout has moved on and the cron hasn't caught up yet. Answer for the
	// clip actually on screen rather than the one that stopped playing.
	vid, err := LoadOrCreate(ctx, file)
	if err != nil {
		slog.DebugContext(ctx, "playhead clip lookup failed", "err", err, "file", file)
		return p.CurrentlyPlaying, p.CurrentProgress()
	}
	return vid, pos
}

// Current returns the currently-playing video.
func (p *Player) Current() Video { return p.CurrentlyPlaying }

// EmitCurrentVideo re-publishes the current clip as a video.changed without a
// transition. cmd calls this once right after the live-console hub subscribes
// to NATS, so a freshly-started hub shows "now playing" immediately instead of
// waiting for the next clip change (NATS core has no replay). No-op when
// nothing is playing yet. A periodic re-emit for a separately-started console
// is the tripbot-console split's concern, not this.
func (p *Player) EmitCurrentVideo(ctx context.Context) {
	if p.CurrentlyPlaying.Slug == "" {
		return
	}
	eventbus.EmitVideoChanged(ctx, p.cfg.Environment, p.cfg.Platform,
		p.CurrentlyPlaying.File(), p.CurrentlyPlaying.State, p.CurrentlyPlaying.Flagged,
		p.CurrentlyPlaying.Lat, p.CurrentlyPlaying.Lng)
}
