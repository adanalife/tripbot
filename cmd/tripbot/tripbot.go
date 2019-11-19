package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/dimiro1/banner/autoload"
	"github.com/dmerrick/danalol-stream/pkg/background"
	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/database"
	"github.com/dmerrick/danalol-stream/pkg/events"
	"github.com/dmerrick/danalol-stream/pkg/tripbot"
	"github.com/dmerrick/danalol-stream/pkg/users"
	"github.com/dmerrick/danalol-stream/pkg/video"
)

func main() {
	// catch CTRL-C and clean up
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Println("caught CTRL-C")
		// anything below this probably wont be executed
		// use !shutdown instead
		events.LogoutAll(tripbot.Uptime)
		log.Printf("last played: %s", video.CurrentlyPlaying)
		users.Shutdown()
		database.DBCon.Close()
		background.StopCron()
		os.Exit(1)
	}()

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
	log.Printf("URL: https://twitch.tv/%s", config.ChannelName)

	// run this right away to set the currently-playing video
	// (otherwise it will be unset until the first cron job runs)
	video.GetCurrentlyPlaying()
	v := video.CurrentlyPlaying
	video.LoadOrCreate(v.String())

	// start cron and attach cronjobs
	background.StartCron()

	//TODO: move these somewhere central
	background.Cron.AddFunc("@every 57m30s", tripbot.Chatter)
	background.Cron.AddFunc("@every 60s", video.GetCurrentlyPlaying)
	background.Cron.AddFunc("@every 60s", users.UpdateSession)

	// actually connect to Twitch
	// wrapped in a loop in case twitch goes down
	for {
		log.Println("Connecting to Twitch...")
		err := client.Connect()
		if err != nil {
			log.Println(err)
			time.Sleep(time.Minute)
		}
	}
}
