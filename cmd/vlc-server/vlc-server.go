package main

import (
	"context"
	"log"
	"log/slog"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	terrors "github.com/adanalife/tripbot/pkg/errors"

	c "github.com/adanalife/tripbot/pkg/config/vlc-server"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/natsclient"
	"github.com/adanalife/tripbot/pkg/obs"
	"github.com/adanalife/tripbot/pkg/telemetry"
	vlcServer "github.com/adanalife/tripbot/pkg/vlc-server"
	"github.com/getsentry/sentry-go"
	"github.com/nats-io/nats.go"
)

// version is overridable at build time via -ldflags "-X main.version=...".
var version = "dev"

var telemetryShutdown telemetry.ShutdownFunc

// srv is the running vlc-server, captured in main so gracefulShutdown can
// reach it from its signal-handler goroutine.
var srv *vlcServer.Server

func main() {
	slog.Info("vlc-server starting", "version", version)

	// we don't yet support libvlc on darwin
	if helpers.RunningOnDarwin() {
		log.Fatal("This doesn't yet work on darwin")
	}

	// create a brand new random seed
	rand.Seed(time.Now().UnixNano())

	// write the current pid to a pidfile
	helpers.WritePidFile(c.Conf.VLCPidFile)

	// shutdownCtx is canceled on SIGINT/SIGTERM; the HTTP server uses it
	// to trigger a graceful shutdown so in-flight requests aren't cut.
	// listenForShutdown's gracefulShutdown goroutine handles the rest of
	// the app cleanup off the same signals.
	shutdownCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

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
		if err := vlcServer.EnsureLastPlayedStream(shutdownCtx, natsclient.JetStream(), c.Conf.Environment); err != nil {
			slog.Warn("lastplayed stream setup failed; resume-on-restart disabled", "err", err)
		}
	})

	// await graceful shutdown signal
	listenForShutdown()

	// set up telemetry (no-op if OTEL_SDK_DISABLED)
	initializeTelemetry()

	// set up error logging
	initializeErrorLogger()

	// start VLC + load media; surfaces libvlc init errors instead of
	// fatalling-during-init from inside the package.
	var err error
	srv, err = vlcServer.New(vlcServer.Config{Version: version})
	if err != nil {
		log.Fatal(err)
	}
	// On normal exit (Start returns when shutdownCtx is canceled), drain
	// the HTTP server with a bounded ctx and release libvlc. The
	// gracefulShutdown goroutine below also calls Shutdown for the
	// os.Exit path — Shutdown tolerates being invoked twice (libvlc
	// Release is a no-op when the instance is already released).
	defer func() {
		drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(drainCtx)
	}()

	pickStartupVideo(shutdownCtx, srv)

	// poll libvlc for playback stats (FPS, bitrate, dropped frames) and
	// surface them as OTel gauges. No-op when telemetry is disabled.
	// Tied to shutdownCtx so the poller goroutine exits cleanly on
	// SIGINT/SIGTERM.
	srv.StartStatsPoller(shutdownCtx, 5*time.Second)

	// keep the JetStream lastplayed last-value cache tracking the playback
	// position, so a restart resumes mid-clip (at worst one interval behind).
	// No-op publishes when NATS is off.
	srv.StartLastPlayedTicker(shutdownCtx, 5*time.Second)

	// poll the OBS WebSocket for streaming state + render/output stats,
	// stamping the series with this instance's streaming platform so the
	// per-platform instances (twitch/youtube/…) stay distinct.
	go obs.PollStreamingActive(context.Background(), c.Conf.Platform, 30*time.Second)

	// Self-heal watchdog: probes the local RTSP listener; after 3
	// consecutive DESCRIBE failures (90s of sustained badness with the
	// 30s interval), writes a resume marker and signals SIGTERM so
	// supervisord respawns vlc-server. See pkg/vlc-server/watchdog.go.
	srv.StartRTSPWatchdog(shutdownCtx, 30*time.Second, 3, 30*time.Second)

	// Cover-frame refresher: re-extracts the next video's first frame
	// whenever the playing video changes. See pkg/vlc-server/firstframe.go.
	srv.StartNextFrameRefresher(shutdownCtx, 5*time.Second)

	// start the webserver
	srv.Start(shutdownCtx)
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

// initializeTelemetry brings up OpenTelemetry providers (traces, metrics,
// logs). No-ops cleanly if OTEL_SDK_DISABLED is set or no OTLP endpoint
// is configured — see pkg/telemetry.
func initializeTelemetry() {
	ctx := context.Background()
	shutdown, err := telemetry.Init(ctx, "vlc-server", version)
	if err != nil {
		slog.WarnContext(ctx, "telemetry init failed", "err", err)
	}
	telemetryShutdown = shutdown
}

// initializeErrorLogger makes sure the logger is configured
func initializeErrorLogger() {
	terrors.Initialize(c.Conf, version)
}

// listenForShutdown creates a background job that listens for a graceful shutdown request
func listenForShutdown() {
	// start the graceful shutdown listener
	go gracefulShutdown()
}

// gracefulShutdown catches CTRL-C and cleans up
func gracefulShutdown() {
	ctrlC := make(chan os.Signal)
	signal.Notify(ctrlC,
		os.Interrupt,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	// wait for signal
	<-ctrlC

	slog.Warn("caught CTRL-C, shutting down")
	if srv != nil {
		drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		srv.Shutdown(drainCtx)
		cancel()
	}
	//TODO: stop cron here
	sentry.Flush(time.Second * 5)
	if telemetryShutdown != nil {
		flushCtx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		if err := telemetryShutdown(flushCtx); err != nil {
			slog.ErrorContext(flushCtx, "telemetry shutdown failed", "err", err)
		}
		cancel()
	}
	os.Exit(1)
}
