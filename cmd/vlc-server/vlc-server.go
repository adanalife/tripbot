package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	terrors "github.com/adanalife/tripbot/pkg/errors"

	c "github.com/adanalife/tripbot/pkg/config/vlc-server"
	"github.com/adanalife/tripbot/pkg/helpers"
	onscreensServer "github.com/adanalife/tripbot/pkg/onscreens-server"
	"github.com/adanalife/tripbot/pkg/telemetry"
	vlcServer "github.com/adanalife/tripbot/pkg/vlc-server"
	"github.com/getsentry/sentry-go"
	"github.com/logrusorgru/aurora/v3"
)

// version is overridable at build time via -ldflags "-X main.version=...".
var version = "dev"

var telemetryShutdown telemetry.ShutdownFunc

func main() {
	log.Println(aurora.Cyan(fmt.Sprintf("vlc-server version %s", version)))

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

	// start the webserver
	vlcServer.SetVersion(version)
	vlcServer.Start()

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
	shutdown, err := telemetry.Init(context.Background(), "vlc-server", version)
	if err != nil {
		log.Println(aurora.Yellow(fmt.Sprintf("telemetry init: %v", err)))
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

	log.Println(aurora.Red("caught CTRL-C"))
	// anything below this probably won't be executed
	vlcServer.Shutdown()
	//TODO: stop cron here
	sentry.Flush(time.Second * 5)
	if telemetryShutdown != nil {
		flushCtx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		if err := telemetryShutdown(flushCtx); err != nil {
			log.Printf("telemetry shutdown: %v", err)
		}
		cancel()
	}
	os.Exit(1)
}
