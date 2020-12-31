package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adanalife/tripbot/pkg/audio"
	"github.com/adanalife/tripbot/pkg/background"
	"github.com/adanalife/tripbot/pkg/chatbot"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/scoreboards"
	"github.com/adanalife/tripbot/pkg/server"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
	"github.com/adanalife/tripbot/pkg/users"
	"github.com/adanalife/tripbot/pkg/video"
	_ "github.com/dimiro1/banner/autoload"
	"github.com/gempir/go-twitch-irc/v2"
	"github.com/getsentry/sentry-go"
	"github.com/logrusorgru/aurora"
)

var client *twitch.Client

// main performs the various steps to get the bot running
func main() {
	createRandomSeed()
	listenForShutdown()
	initializeErrorLogger()
	startHttpServer()
	findInitialVideo()
	users.InitLeaderboard()
	scoreboards.TopUsers(scoreboards.CurrentMilesScoreboard())
	scoreboards.TopUsers(scoreboards.CurrentGuessScoreboard())
	startCron()
	setUpTwitchClient() // required for the below
	updateSubscribers()
	getCurrentUsers()
	updateWebhookSubscriptions()
	connectToTwitch()
}

// createRandomSeed ensures that random numbers will be random
func createRandomSeed() {
	// create a brand new random seed
	rand.Seed(time.Now().UnixNano())
}

// listenForShutdown creates a background job that listens for a graceful shutdown request
func listenForShutdown() {
	helpers.WritePidFile(c.Conf.TripbotPidFile)
	// start the graceful shutdown listener
	go gracefulShutdown()
}

// initializeErrorLogger makes sure the logger is configured
func initializeErrorLogger() {
	terrors.Initialize(c.Conf)
}

// startHttpServer starts a webserver, which is
// used for admin tools and receiving webhooks
func startHttpServer() {
	// start the HTTP server
	go server.Start()
}

// findInitialVideo will determin the vido that is currently-playing
// we want to run this early, otherwise it will be unset until the first cron job runs
func findInitialVideo() {
	video.GetCurrentlyPlaying()
	v := video.CurrentlyPlaying
	_, err := video.LoadOrCreate(v.String())
	if err != nil {
		terrors.Log(err, "error loading initial video, is there a video playing?")
	}
}

// startCron starts the background workers
func startCron() {
	// start cron and attach cronjobs
	background.StartCron()
	scheduleBackgroundJobs()
}

// setUpTwitchClient sets up the Twitch client,
// used by many bot features
func setUpTwitchClient() {
	// set up the Twitch client
	client = chatbot.Initialize()
}

// updateSubscribers gets the list of current subscribers
func updateSubscribers() {
	// update subscribers list
	mytwitch.GetSubscribers()
}

// getCurrentUsers gets the users watching the stream
func getCurrentUsers() {
	// fetch initial session
	users.UpdateSession()
	users.PrintCurrentSession()
}

//updateWebhookSubscriptions makes sure webhooks are being sent to the bot
func updateWebhookSubscriptions() {
	// create webhook subscriptions
	mytwitch.UpdateWebhookSubscriptions()
}

// connectToTwitch joins Twitch chat and starts listening
func connectToTwitch() {
	client.Join(c.Conf.ChannelName)
	log.Println("Joined channel", c.Conf.ChannelName)
	log.Printf("URL: %s", aurora.Blue(fmt.Sprintf("https://twitch.tv/%s", c.Conf.ChannelName)).Underline())

	// actually connect to Twitch
	// wrapped in a loop in case twitch goes down
	for {
		log.Println(aurora.Magenta("Initializing connection to Twitch"))
		err := client.Connect()
		if err != nil {
			terrors.Log(err, "unable to connect to twitch")
			time.Sleep(time.Minute)
		}
	}
}

// gracefulShutdown catches CTRL-C and cleans up
func gracefulShutdown() {
	ctrlC := make(chan os.Signal)
	signal.Notify(ctrlC, os.Interrupt, syscall.SIGTERM)

	// wait for signal
	<-ctrlC

	log.Println(aurora.Red("caught CTRL-C"))
	// anything below this probably wont be executed
	// try and use !shutdown instead
	//TODO: print different message if CurrentlyPlaying is ""
	log.Printf("Last played video: %s", aurora.Yellow(video.CurrentlyPlaying.File()))
	users.Shutdown()
	err := database.Connection().Close()
	if err != nil {
		terrors.Log(err, "error closing DB connection")
	}
	background.StopCron()
	audio.Shutdown()
	sentry.Flush(time.Second * 5)
	os.Exit(1)
}

// scheduleBackgroundJobs schedules the various background jobs
// the reason we put this is in this package is because adding this to background
// would cause circular dependencies
func scheduleBackgroundJobs() {
	var err error

	// schedule these functions
	err = background.Cron.AddFunc("@every 60s", video.GetCurrentlyPlaying)
	// use this to keep the connection to MPD running
	err = background.Cron.AddFunc("@every 60s", audio.RefreshClient)
	err = background.Cron.AddFunc("@every 61s", users.UpdateSession)
	// err = background.Cron.AddFunc("@every 62s", users.UpdateLeaderboard)
	err = background.Cron.AddFunc("@every 5m", users.PrintCurrentSession)
	err = background.Cron.AddFunc("@every 15m", mytwitch.GetSubscribers)
	err = background.Cron.AddFunc("@every 1h", mytwitch.RefreshUserAccessToken)
	err = background.Cron.AddFunc("@every 2h57m30s", chatbot.Chatter)
	err = background.Cron.AddFunc("@every 12h", mytwitch.UpdateWebhookSubscriptions)
	if helpers.RunningOnDarwin() {
		err = background.Cron.AddFunc("@every 6h", audio.RestartItunes)
	}
	if !helpers.RunningOnWindows() {
		err = background.Cron.AddFunc("@every 12h", mytwitch.SetStreamTags)
	}

	if err != nil {
		terrors.Log(err, "error adding at least one background job!")
	}
}
