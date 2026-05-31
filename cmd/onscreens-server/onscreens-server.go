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
	"github.com/adanalife/tripbot/pkg/natsclient"
	onscreensServer "github.com/adanalife/tripbot/pkg/onscreens-server"
	"github.com/adanalife/tripbot/pkg/telemetry"
	"github.com/getsentry/sentry-go"
)

// version is overridable at build time via -ldflags "-X main.version=...".
var version = "dev"

var telemetryShutdown telemetry.ShutdownFunc

// srv is the running onscreens-server, captured in main so
// gracefulShutdown can reach it from its signal-handler goroutine.
var srv *onscreensServer.Server

// httpShutdownTimeout is how long gracefulShutdown waits for in-flight
// requests to finish before forcing connections closed. 5s matches the
// telemetry/sentry flush deadlines used below — the whole shutdown path
// should complete in well under 15s so process supervisors don't SIGKILL
// us mid-drain.
const httpShutdownTimeout = 5 * time.Second

func main() {
	slog.Info("onscreens-server starting", "version", version)

	// write the current pid to a pidfile
	helpers.WritePidFile(c.Conf.OnscreensPidFile)

	// shutdownCtx is canceled on SIGINT/SIGTERM; the HTTP server uses it
	// to trigger a graceful shutdown so in-flight requests aren't cut.
	// listenForShutdown's gracefulShutdown goroutine handles the rest of
	// the app cleanup off the same signals.
	shutdownCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	// Connect to NATS so Server.Start can attach subscribers. Optional —
	// when NATS_URL is empty the conn is nil and the subscriber registration
	// is skipped; HTTP remains the sole transport.
	natsclient.Connect(c.Conf.NatsURL, "onscreens-server")

	// construct the server — runs all per-onscreen init (singletons +
	// background loops) up front so the HTTP routes have everything to
	// read by the time the listener accepts.
	srv = onscreensServer.New(onscreensServer.Config{Version: version})

	// await graceful shutdown signal
	listenForShutdown()

	// set up telemetry (no-op if OTEL_SDK_DISABLED)
	initializeTelemetry()

	// set up error logging
	initializeErrorLogger()

	// start the webserver — blocks until ListenAndServe returns or the
	// signal handler cancels shutdownCtx
	if err := srv.Start(shutdownCtx); err != nil {
		terrors.Fatal(err, "couldn't start server")
	}
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

// gracefulShutdown catches CTRL-C and cleans up. Drains the HTTP server
// first (so in-flight onscreens-render requests don't get cut), then
// flushes Sentry + telemetry exporters before exiting.
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

	if srv != nil {
		httpCtx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
		if err := srv.Shutdown(httpCtx); err != nil {
			slog.Error("error during onscreens-server HTTP shutdown", "err", err)
		}
		cancel()
	}

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
