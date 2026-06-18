package video

import (
	"context"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/eventbus"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	vlcClient "github.com/adanalife/tripbot/pkg/vlc-client"
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
// drive the VLC playback + onscreens overlays. Construct via NewPlayer; the
// single process-wide instance lives on cmd/tripbot's Tripbot struct.
type Player struct {
	CurrentlyPlaying Video // exported because external callers read the current video off it
	curVid, preVid   string
	timeStarted      time.Time
	onscreens        onscreens
	vlc              *vlcClient.Client
}

// NewPlayer returns a Player with its own Onscreens + VLC clients.
func NewPlayer(onscreens onscreens, vlc *vlcClient.Client) *Player {
	return &Player{onscreens: onscreens, vlc: vlc}
}

// GetCurrentlyPlaying will use lsof to figure out
// which dashcam video is currently playing (seriously).
// ctx is forward-compat plumbing — vlc-client and onscreens-client don't
// take ctx yet, so it's not propagated into their HTTP calls. Once they do,
// trace spans for cron.video.GetCurrentlyPlaying ticks will nest the
// underlying VLC poll and GPS-image toggles as children.
// TODO: consider making this return a video struct
func (p *Player) GetCurrentlyPlaying(ctx context.Context) {
	var err error

	// save the video we used last time
	p.preVid = p.curVid

	// figure out what's currently playing
	if helpers.RunningOnDarwin() {
		p.curVid = p.figureOutCurrentVideo(ctx)
	} else {
		p.curVid = p.vlc.CurrentlyPlaying(ctx)
	}

	// if the currently-playing video has changed
	if p.curVid != p.preVid {
		// reset the stopwatch
		p.timeStarted = time.Now()

		// share the Video with the system
		p.CurrentlyPlaying, err = LoadOrCreate(ctx, p.curVid)
		if err != nil {
			// Downstream of vlc.CurrentlyPlaying; the wrapper there already
			// logged the root cause at Error. Debug-level keeps the breadcrumb
			// without double-counting in Sentry.
			slog.DebugContext(ctx, "unable to create Video", "err", err, "file", p.curVid)
		}

		slog.InfoContext(ctx, "now playing",
			"file", p.CurrentlyPlaying.File(),
			"state", helpers.StateToStateAbbrev(p.CurrentlyPlaying.State),
		)

		// Update the current-state gauge: set the new state's series to 1 and
		// clear the prior one. A blank abbrev (unresolvable state) records as
		// "unknown" so a stuck playhead is alertable.
		instrumentation.CurrentState.Set(helpers.StateToStateAbbrev(p.CurrentlyPlaying.State))

		// Announce the switch so the admin panel's "now playing" card updates
		// live (no-op when NATS is unconfigured). emitted_at doubles as the
		// clip start time for the panel's elapsed ticker.
		eventbus.EmitVideoChanged(ctx, c.Conf.Environment, c.Conf.Platform,
			p.CurrentlyPlaying.File(), p.CurrentlyPlaying.State, p.CurrentlyPlaying.Flagged,
			p.CurrentlyPlaying.Lat, p.CurrentlyPlaying.Lng)

		// show the no-GPS image
		if p.CurrentlyPlaying.Flagged {
			//TODO: kinda cludgy that we hardcode 60s here
			p.onscreens.ShowGPSImage(ctx, 60*time.Second)
		} else {
			p.onscreens.HideGPSImage(ctx)
		}
	}
}

// CurrentProgress represents how long the video has been playing
// it will be useful eventually for choosing the exact right screenshot
func (p *Player) CurrentProgress() time.Duration {
	return time.Since(p.timeStarted)
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
	eventbus.EmitVideoChanged(ctx, c.Conf.Environment, c.Conf.Platform,
		p.CurrentlyPlaying.File(), p.CurrentlyPlaying.State, p.CurrentlyPlaying.Flagged,
		p.CurrentlyPlaying.Lat, p.CurrentlyPlaying.Lng)
}

func (p *Player) figureOutCurrentVideo(ctx context.Context) string {
	if helpers.RunningOnWindows() {
		slog.ErrorContext(ctx, "can't run script on windows")
		return ""
	}
	// run the shell script to get currently-playing video
	scriptPath := filepath.Join(helpers.ProjectRoot(), "bin", "current-file.sh")
	out, err := exec.Command(scriptPath).Output()
	outString := strings.TrimSpace(string(out))
	if err != nil {
		slog.ErrorContext(ctx, "figureOutCurrentVideo script failed", "err", err, "output", outString)
		return ""
	}
	return outString
}
