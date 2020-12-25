package main

import (
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/vlc-server"
	"github.com/adanalife/tripbot/pkg/helpers"
	onscreensServer "github.com/adanalife/tripbot/pkg/onscreens-server"
	vlcServer "github.com/adanalife/tripbot/pkg/vlc-server"
	"github.com/getsentry/sentry-go"
	"github.com/logrusorgru/aurora"
)

func main() {
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

	// start VLC
	vlcServer.InitPlayer()
	vlcServer.PlayRandom() // play a random video

	// start the webserver
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
	// anything below this probably wont be executed
	vlcServer.Shutdown()
	//TODO: stop cron here
	sentry.Flush(time.Second * 5)
	os.Exit(1)
}
