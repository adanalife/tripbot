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
	"github.com/adanalife/tripbot/pkg/obs"
	onscreensServer "github.com/adanalife/tripbot/pkg/onscreens-server"
	"github.com/adanalife/tripbot/pkg/telemetry"
	vlcServer "github.com/adanalife/tripbot/pkg/vlc-server"
	"github.com/getsentry/sentry-go"
)

// version is overridable at build time via -ldflags "-X main.version=...".
var version = "dev"

var telemetryShutdown telemetry.ShutdownFunc

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

	// initialize the onscreen elements
	createOnscreens()

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

	// start VLC
	vlcServer.InitPlayer()
	vlcServer.PlayRandom() // play a random video

	// poll libvlc for playback stats (FPS, bitrate, dropped frames) and
	// surface them as OTel gauges. No-op when telemetry is disabled.
	vlcServer.StartStatsPoller(context.Background(), 5*time.Second)

	// poll the OBS WebSocket for streaming state + render/output stats.
	go obs.PollStreamingActive(context.Background(), 30*time.Second)

	// start the webserver
	vlcServer.SetVersion(version)
	vlcServer.Start(shutdownCtx)

	// listen for termination signals and gracefully shutdown
	defer vlcServer.Shutdown()
}

// createOnscreens starts the various onscreen elements
// (like the chat boxes in the corners)
func createOnscreens() {
	onscreensServer.InitGPSImage()
	onscreensServer.InitLeftRotator()
	onscreensServer.InitRightRotator()
	onscreensServer.InitMiddleText()
	onscreensServer.InitTimewarp()
	onscreensServer.InitLeaderboard()
	onscreensServer.InitFlagImage()
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
	vlcServer.Shutdown()
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
