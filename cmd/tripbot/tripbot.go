package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/dimiro1/banner/autoload"
	"github.com/dmerrick/danalol-stream/pkg/background"
	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/database"
	"github.com/dmerrick/danalol-stream/pkg/server"
	"github.com/dmerrick/danalol-stream/pkg/tripbot"
	mytwitch "github.com/dmerrick/danalol-stream/pkg/twitch"
	"github.com/dmerrick/danalol-stream/pkg/users"
	"github.com/dmerrick/danalol-stream/pkg/video"
	"github.com/getsentry/sentry-go"
	"github.com/logrusorgru/aurora"
)

func main() {
	// catch CTRL-C and clean up
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Println("caught CTRL-C")
		// anything below this probably wont be executed
		// try and use !shutdown instead
		log.Printf("last played: %s", video.CurrentlyPlaying)
		users.Shutdown()
		database.DBCon.Close()
		background.StopCron()
		sentry.Flush(time.Second * 5)
		os.Exit(1)
	}()

	// start the HTTP server
	go server.Start()

	// set up the Twitch client
	client := tripbot.Initialize()

	// attach handlers
	client.OnUserJoinMessage(tripbot.UserJoin)
	client.OnUserPartMessage(tripbot.UserPart)
	client.OnUserNoticeMessage(tripbot.UserNotice)
	client.OnWhisperMessage(tripbot.Whisper)
	client.OnPrivateMessage(tripbot.PrivateMessage)

	// join the channel
	client.Join(config.ChannelName)
	log.Println("Joined channel", config.ChannelName)
	log.Printf("URL: %s", aurora.Underline(aurora.Blue(fmt.Sprintf("https://twitch.tv/%s", config.ChannelName))))

	// run this right away to set the currently-playing video
	// (otherwise it will be unset until the first cron job runs)
	video.GetCurrentlyPlaying()
	v := video.CurrentlyPlaying
	video.LoadOrCreate(v.String())

	// initialize the leaderboard
	users.InitLeaderboard()

	// start cron and attach cronjobs
	background.StartCron()

	// update subscribers list
	mytwitch.GetSubscribers()

	// fetch initial session
	users.UpdateSession()
	users.PrintCurrentSession()

	// create webhook subscriptions
	mytwitch.UpdateWebhookSubscriptions()

	//TODO: move these somewhere central
	background.Cron.AddFunc("@every 60s", video.GetCurrentlyPlaying)
	background.Cron.AddFunc("@every 61s", users.UpdateSession)
	background.Cron.AddFunc("@every 62s", users.UpdateLeaderboard)
	background.Cron.AddFunc("@every 5m", users.PrintCurrentSession)
	background.Cron.AddFunc("@every 15m", mytwitch.GetSubscribers)
	background.Cron.AddFunc("@every 1h", mytwitch.RefreshUserAccessToken)
	background.Cron.AddFunc("@every 57m30s", tripbot.Chatter)
	background.Cron.AddFunc("@every 12h", mytwitch.SetStreamTags)
	background.Cron.AddFunc("@every 12h", mytwitch.UpdateWebhookSubscriptions)

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
