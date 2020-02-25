package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	_ "github.com/dimiro1/banner/autoload"
	"github.com/dmerrick/danalol-stream/pkg/audio"
	"github.com/dmerrick/danalol-stream/pkg/background"
	"github.com/dmerrick/danalol-stream/pkg/chatbot"
	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/database"
	"github.com/dmerrick/danalol-stream/pkg/server"
	mytwitch "github.com/dmerrick/danalol-stream/pkg/twitch"
	"github.com/dmerrick/danalol-stream/pkg/users"
	"github.com/dmerrick/danalol-stream/pkg/video"
	"github.com/dmerrick/danalol-stream/pkg/vlc"
	"github.com/getsentry/sentry-go"
	"github.com/logrusorgru/aurora"
)

func main() {
	// start the graceful shutdown listener
	go gracefulShutdown()

	// start the HTTP server
	go server.Start()

	// set up the Twitch client
	client := chatbot.Initialize()

	if runtime.GOOS != "darwin" {
		// start VLC
		vlc.Init()
		vlc.PlayRandom()
	}

	// run this right away to set the currently-playing video
	// (otherwise it will be unset until the first cron job runs)
	background.InitGPSImage() // this has to happen first
	video.GetCurrentlyPlaying()
	v := video.CurrentlyPlaying
	video.LoadOrCreate(v.String())

	// initialize the leaderboard
	users.InitLeaderboard()

	// start cron and attach cronjobs
	background.StartCron()
	scheduleBackgroundJobs()

	// update subscribers list
	mytwitch.GetSubscribers()

	// fetch initial session
	users.UpdateSession()
	users.PrintCurrentSession()

	// create webhook subscriptions
	mytwitch.UpdateWebhookSubscriptions()

	background.InitChat()
	background.InitLeaderboard()
	background.InitLeftRotator()
	background.InitRightRotator()

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

// catch CTRL-C and clean up
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
	database.DBCon.Close()
	background.StopCron()
	audio.Shutdown()
	vlc.Shutdown()
	sentry.Flush(time.Second * 5)
	os.Exit(1)
}

// the reason we put this here is because adding this to background
// would cause circular dependencies
func scheduleBackgroundJobs() {
	// schedule these functions
	background.Cron.AddFunc("@every 60s", video.GetCurrentlyPlaying)
	// use this to keep the connection to MPD running
	background.Cron.AddFunc("@every 60s", audio.RefreshClient)
	background.Cron.AddFunc("@every 61s", users.UpdateSession)
	background.Cron.AddFunc("@every 62s", users.UpdateLeaderboard)
	background.Cron.AddFunc("@every 5m", users.PrintCurrentSession)
	background.Cron.AddFunc("@every 15m", mytwitch.GetSubscribers)
	background.Cron.AddFunc("@every 1h", mytwitch.RefreshUserAccessToken)
	background.Cron.AddFunc("@every 2h57m30s", chatbot.Chatter)
	background.Cron.AddFunc("@every 12h", mytwitch.SetStreamTags)
	background.Cron.AddFunc("@every 12h", mytwitch.UpdateWebhookSubscriptions)
}
