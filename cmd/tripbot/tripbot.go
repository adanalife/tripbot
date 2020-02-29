package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/dimiro1/banner/autoload"
	"github.com/dmerrick/danalol-stream/pkg/audio"
	"github.com/dmerrick/danalol-stream/pkg/background"
	"github.com/dmerrick/danalol-stream/pkg/chatbot"
	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/database"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/server"
	mytwitch "github.com/dmerrick/danalol-stream/pkg/twitch"
	"github.com/dmerrick/danalol-stream/pkg/users"
	"github.com/dmerrick/danalol-stream/pkg/video"
	"github.com/gempir/go-twitch-irc/v2"
	"github.com/getsentry/sentry-go"
	"github.com/logrusorgru/aurora"
)

var client *twitch.Client

// main performs the various steps to get the bot running
func main() {
	createRandomSeed()
	listenForShutdown()
	startHttpServer()
	findInitialVideo()
	setUpLeaderboard()
	startCron()
	setUpTwitchClient() // required for the below
	updateSubscribers()
	getCurrentUsers()
	updateWebhookSubscriptions()
	createOnscreens()
	connectToTwitch()
}

// createRandomSeed ensures that random numbers will be random
func createRandomSeed() {
	// create a brand new random seed
	rand.Seed(time.Now().UnixNano())
}

// listenForShutdown creates a background job that listens for a graceful shutdown request
func listenForShutdown() {
	// start the graceful shutdown listener
	go gracefulShutdown()
}

// startHttpServer starts a webserver, which is
// used for admin tools and recieving webhooks
func startHttpServer() {
	// start the HTTP server
	go server.Start()
}

// findInitialVideo will determin the vido that is currently-playing
// we want to run this early, otherwise it will be unset until the first cron job runs
func findInitialVideo() {
	background.InitGPSImage() // this has to happen first
	video.GetCurrentlyPlaying()
	v := video.CurrentlyPlaying
	_, err := video.LoadOrCreate(v.String())
	if err != nil {
		terrors.Log(err, "error loading initial video, is VLC running?")
	}
}

// setUpLeaderboard figures out the current leaderboard
// and displays the oscreen for it
func setUpLeaderboard() {
	// initialize the leaderboard
	users.InitLeaderboard()
	background.InitLeaderboard()
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

// createOnscreens starts the various onscreen elements
// (like the chat boxes in the corners)
func createOnscreens() {
	background.InitChat()
	background.InitLeftRotator()
	background.InitRightRotator()
	background.InitMiddleText()
	background.InitTimewarp()
}

// connectToTwitch joins Twitch chat and starts listening
func connectToTwitch() {
	client.Join(config.ChannelName)
	log.Println("Joined channel", config.ChannelName)
	log.Printf("URL: %s", aurora.Blue(fmt.Sprintf("https://twitch.tv/%s", config.ChannelName)).Underline())

	// actually connect to Twitch
	// wrapped in a loop in case twitch goes down
	for {
		log.Println("Connecting to Twitch")
		err := client.Connect()
		if err != nil {
			log.Println(err)
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
	log.Printf("last played: %s", video.CurrentlyPlaying)
	users.Shutdown()
	err := database.DBCon.Close()
	if err != nil {
		terrors.Log(err, "error closing DB connection")
	}
	background.StopCron()
	audio.Shutdown()
	sentry.Flush(time.Second * 5)
	os.Exit(1)
}

// scheduleBackgroundJobs schedules the various backgroun jobs
// the reason we put this is in this package is because adding this to background
// would cause circular dependencies
func scheduleBackgroundJobs() {
	var err error

	// schedule these functions
	err = background.Cron.AddFunc("@every 60s", video.GetCurrentlyPlaying)
	// use this to keep the connection to MPD running
	err = background.Cron.AddFunc("@every 60s", audio.RefreshClient)
	err = background.Cron.AddFunc("@every 61s", users.UpdateSession)
	err = background.Cron.AddFunc("@every 62s", users.UpdateLeaderboard)
	err = background.Cron.AddFunc("@every 5m", users.PrintCurrentSession)
	err = background.Cron.AddFunc("@every 15m", mytwitch.GetSubscribers)
	err = background.Cron.AddFunc("@every 1h", mytwitch.RefreshUserAccessToken)
	err = background.Cron.AddFunc("@every 2h57m30s", chatbot.Chatter)
	err = background.Cron.AddFunc("@every 12h", mytwitch.SetStreamTags)
	err = background.Cron.AddFunc("@every 12h", mytwitch.UpdateWebhookSubscriptions)
	if helpers.RunningOnDarwin() {
		err = background.Cron.AddFunc("@every 6h", audio.RestartItunes)
	}

	if err != nil {
		terrors.Log(err, "error adding at least one background job!")
	}
}
