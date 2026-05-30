package video

import (
	"context"
	"log/slog"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/eventbus"
	"github.com/adanalife/tripbot/pkg/helpers"
	onscreensClient "github.com/adanalife/tripbot/pkg/onscreens-client"
	vlcClient "github.com/adanalife/tripbot/pkg/vlc-client"
)

// Player owns the state of "what's currently playing" and the clients that
// drive the VLC playback + onscreens overlays. Construct via NewPlayer; the
// package-level defaultPlayer is wired up at init for callers that still hit
// the free-function shims below.
type Player struct {
	CurrentlyPlaying Video // exported because external callers used to read video.CurrentlyPlaying
	curVid, preVid   string
	timeStarted      time.Time
	onscreens        *onscreensClient.Client
	vlc              *vlcClient.Client
}

// NewPlayer returns a Player with its own Onscreens + VLC clients.
func NewPlayer(onscreens *onscreensClient.Client, vlc *vlcClient.Client) *Player {
	return &Player{onscreens: onscreens, vlc: vlc}
}

// defaultPlayer is the package-level Player used by the free-function shims
// below. Exists so callers that aren't constructor-injected (cmd/tripbot
// bootstrap, script/collect-gps) keep working. New consumers should construct
// their own *Player via NewPlayer().
var defaultPlayer = NewPlayer(
	onscreensClient.New(c.Conf.OnscreensServerHost),
	vlcClient.New(c.Conf.VlcServerHost),
)

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

		// Announce the switch so the admin panel's "now playing" card updates
		// live (no-op when NATS is unconfigured). emitted_at doubles as the
		// clip start time for the panel's elapsed ticker.
		eventbus.EmitVideoChanged(ctx, c.Conf.Environment,
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

func (p *Player) figureOutCurrentVideo(ctx context.Context) string {
	if helpers.RunningOnWindows() {
		slog.ErrorContext(ctx, "can't find current video on windows")
		return ""
	}
	file, err := currentVideoFile()
	if err != nil {
		slog.ErrorContext(ctx, "figureOutCurrentVideo failed", "err", err)
		return ""
	}
	return file
}

// ---- package-level shims (transitional) ----
// Each free function calls the corresponding method on defaultPlayer. These
// preserve the existing public surface for unmigrated callers. New consumers
// should construct their own *Player via NewPlayer().

func GetCurrentlyPlaying(ctx context.Context) { defaultPlayer.GetCurrentlyPlaying(ctx) }
func CurrentProgress() time.Duration          { return defaultPlayer.CurrentProgress() }
func CurrentlyPlaying() Video                 { return defaultPlayer.Current() }
