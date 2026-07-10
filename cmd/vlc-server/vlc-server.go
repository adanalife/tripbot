package main

import (
	"context"
	"log"
	"log/slog"
	"math/rand"
	"os"
	"time"

	"github.com/adanalife/tripbot/pkg/bootstrap"
	c "github.com/adanalife/tripbot/pkg/config/vlc-server"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/adanalife/tripbot/pkg/natsclient"
	"github.com/adanalife/tripbot/pkg/obs"
	vlcServer "github.com/adanalife/tripbot/pkg/vlc-server"
	"github.com/nats-io/nats.go"
)

// version is overridable at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	slog.Info("vlc-server starting", "version", version)

	// we don't yet support libvlc on darwin
	if helpers.RunningOnDarwin() {
		log.Fatal("This doesn't yet work on darwin")
	}

	// create a brand new random seed
	rand.Seed(time.Now().UnixNano())

	// ctx is canceled on SIGINT/SIGTERM; every background goroutine hangs
	// off it, srv.Start returns when it cancels, the deferred drain runs,
	// and the process exits 0. There is no separate signal-handler
	// goroutine — this is the only shutdown path.
	ctx, flush := bootstrap.Start("vlc-server", version, c.Conf)
	defer flush()

	// Connect to NATS so Server.Start can attach the command subscribers.
	// Optional — when NATS_URL is empty the conn is nil and the subscriber
	// registration is skipped; HTTP remains the sole transport. A failed dial
	// retries in the background (boot race) with subscriptions replayed on
	// connect, so the subscribers below bind either way.
	//
	// The lastplayed last-value-cache stream is declared in the on-connect
	// callback — a core publish to a subject no stream covers is silently
	// uncaptured, and declaring on connect works even when the client wins
	// the boot race and connects late. Best-effort: resume-on-restart
	// degrades to PlayRandom without it.
	natsclient.Connect(c.Conf.NatsURL, "vlc-server", func(*nats.Conn) {
		if err := vlcServer.EnsureLastPlayedStream(ctx, natsclient.JetStream(), c.Conf.Environment); err != nil {
			slog.Warn("lastplayed stream setup failed; resume-on-restart disabled", "err", err)
		}
	})

	// stamp the streaming platform onto the vlc-server/OBS gauges so the
	// per-platform instances (twitch/youtube/…) stay distinct series instead
	// of colliding on identical labels. Must run before the stats pollers.
	instrumentation.SetPlatform(c.Conf.Platform)

	// start VLC + load media; surfaces libvlc init errors instead of
	// fatalling-during-init from inside the package.
	srv, err := vlcServer.New(vlcServer.Config{Version: version})
	if err != nil {
		log.Fatal(err)
	}
	// Once Start returns (ctx canceled or listen failure), drain the HTTP
	// server with a bounded ctx and release libvlc — before the deferred
	// flush above sends the exporter backlog.
	defer func() {
		drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(drainCtx)
	}()

	pickStartupVideo(ctx, srv)

	// poll libvlc for playback stats (FPS, bitrate, dropped frames) and
	// surface them as OTel gauges. No-op when telemetry is disabled.
	// Tied to ctx so the poller goroutine exits cleanly on SIGINT/SIGTERM.
	srv.StartStatsPoller(ctx, 5*time.Second)

	// keep the JetStream lastplayed last-value cache tracking the playback
	// position, so a restart resumes mid-clip (at worst one interval behind).
	// No-op publishes when NATS is off.
	srv.StartLastPlayedTicker(ctx, 5*time.Second)

	// poll the OBS WebSocket for streaming state + render/output stats.
	go obs.PollStreamingActive(ctx, 30*time.Second)

	// Self-heal watchdog: probes the local RTSP listener; after 3
	// consecutive DESCRIBE failures (90s of sustained badness with the
	// 30s interval), writes a resume marker and signals SIGTERM so the
	// pod's restartPolicy respawns vlc-server. See pkg/vlc-server/watchdog.go.
	srv.StartRTSPWatchdog(ctx, 30*time.Second, 3, 30*time.Second)

	// Cover-frame refresher: re-extracts the next video's first frame
	// whenever the playing video changes. See pkg/vlc-server/firstframe.go.
	srv.StartNextFrameRefresher(ctx, 5*time.Second)

	// start the webserver — blocks until the signal context cancels
	srv.Start(ctx)
}

// pickStartupVideo picks the startup video, trying the most specific resume
// signal first:
//
//  1. the watchdog's file marker (written just before a self-heal SIGTERM —
//     same pod, freshest signal, consumed on read),
//  2. the JetStream lastplayed last-value cache (survives pod restarts and
//     reschedules; per-platform leaf so the twitch and youtube instances
//     each resume their own clip),
//  3. a fresh random pick.
func pickStartupVideo(ctx context.Context, srv *vlcServer.Server) {
	if resumeFromMarker(ctx, srv) {
		return
	}
	if srv.ResumeFromLastPlayed(ctx) {
		return
	}
	srv.PlayRandom()
}

// resumeFromMarker resumes from the self-heal watchdog's file marker, if one
// exists. The marker is consumed on read so a crash-loop doesn't pin playback
// to the same broken clip forever. Returns false when there's no marker (the
// common case) or the marked file can't be played.
func resumeFromMarker(ctx context.Context, srv *vlcServer.Server) bool {
	marker := vlcServer.ResumeMarkerPath()
	data, err := os.ReadFile(marker)
	if err != nil {
		return false
	}
	// Remove the marker immediately so any subsequent crash falls back to
	// the next startup pick rather than retrying the same file.
	if rmErr := os.Remove(marker); rmErr != nil {
		slog.Warn("failed to remove resume marker", "err", rmErr, "marker", marker)
	}
	basename, posMs := vlcServer.ParseResumeMarker(data)
	if basename == "" {
		return false
	}
	slog.Info("resuming playback from watchdog marker", "video", basename, "position_ms", posMs, "marker", marker)
	if err := srv.PlayVideoFile(basename); err != nil {
		slog.Error("marker resume failed; falling back", "err", err, "video", basename)
		return false
	}
	srv.SeekToPosition(ctx, posMs)
	return true
}
