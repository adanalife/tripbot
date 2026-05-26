package main

import (
	"context"
	"log"
	"log/slog"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	terrors "github.com/adanalife/tripbot/pkg/errors"

	c "github.com/adanalife/tripbot/pkg/config/vlc-server"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/obs"
	"github.com/adanalife/tripbot/pkg/telemetry"
	vlcServer "github.com/adanalife/tripbot/pkg/vlc-server"
	"github.com/getsentry/sentry-go"
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

	resumeFromMarker(srv)

	// poll libvlc for playback stats (FPS, bitrate, dropped frames) and
	// surface them as OTel gauges. No-op when telemetry is disabled.
	// Tied to shutdownCtx so the poller goroutine exits cleanly on
	// SIGINT/SIGTERM.
	srv.StartStatsPoller(shutdownCtx, 5*time.Second)

	// poll the OBS WebSocket for streaming state + render/output stats.
	go obs.PollStreamingActive(context.Background(), 30*time.Second)

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

// resumeFromMarker picks the startup video. If the self-heal watchdog
// wrote a resume marker on its previous exit, play that file; otherwise
// start fresh with a random pick. The marker is consumed on read so a
// crash-loop doesn't pin playback to the same broken clip forever.
func resumeFromMarker(srv *vlcServer.Server) {
	marker := vlcServer.ResumeMarkerPath()
	data, err := os.ReadFile(marker)
	if err != nil {
		srv.PlayRandom()
		return
	}
	// Remove the marker immediately so any subsequent crash falls back to
	// PlayRandom rather than retrying the same file.
	if rmErr := os.Remove(marker); rmErr != nil {
		slog.Warn("failed to remove resume marker", "err", rmErr, "marker", marker)
	}
	basename := strings.TrimSpace(string(data))
	if basename == "" {
		srv.PlayRandom()
		return
	}
	slog.Info("resuming playback from watchdog marker", "video", basename, "marker", marker)
	if err := srv.PlayVideoFile(basename); err != nil {
		slog.Error("resume failed; falling back to PlayRandom", "err", err, "video", basename)
		srv.PlayRandom()
	}
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
	// anything below this probably won't be executed
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
