package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/onscreens-server"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
	onscreensServer "github.com/adanalife/tripbot/pkg/onscreens-server"
	"github.com/adanalife/tripbot/pkg/telemetry"
	"github.com/getsentry/sentry-go"
)

// version is overridable at build time via -ldflags "-X main.version=...".
var version = "dev"

var telemetryShutdown telemetry.ShutdownFunc

func main() {
	slog.Info("onscreens-server starting", "version", version)

	// write the current pid to a pidfile
	helpers.WritePidFile(c.Conf.OnscreensPidFile)

	// initialize the onscreen elements (singletons + their background loops)
	createOnscreens()

	// await graceful shutdown signal
	listenForShutdown()

	// set up telemetry (no-op if OTEL_SDK_DISABLED)
	initializeTelemetry()

	// set up error logging
	initializeErrorLogger()

	// start the webserver
	onscreensServer.SetVersion(version)
	onscreensServer.Start()
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
	shutdown, err := telemetry.Init(context.Background(), "onscreens-server", version)
	if err != nil {
		slog.Warn("telemetry init failed", "err", err)
	}
	telemetryShutdown = shutdown
}

// initializeErrorLogger makes sure the logger is configured
func initializeErrorLogger() {
	terrors.Initialize(c.Conf, version)
}

// listenForShutdown creates a background job that listens for a graceful shutdown request
func listenForShutdown() {
	go gracefulShutdown()
}

// gracefulShutdown catches CTRL-C and cleans up
func gracefulShutdown() {
	ctrlC := make(chan os.Signal, 1)
	signal.Notify(ctrlC,
		os.Interrupt,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	<-ctrlC

	slog.Warn("caught CTRL-C, shutting down")
	sentry.Flush(time.Second * 5)
	if telemetryShutdown != nil {
		flushCtx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		if err := telemetryShutdown(flushCtx); err != nil {
			slog.Error("telemetry shutdown failed", "err", err)
		}
		cancel()
	}
	os.Exit(1)
}
