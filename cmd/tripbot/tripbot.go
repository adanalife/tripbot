package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/dimiro1/banner/autoload"
	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/database"
	"github.com/dmerrick/danalol-stream/pkg/events"
	"github.com/dmerrick/danalol-stream/pkg/tripbot"
	"github.com/dmerrick/danalol-stream/pkg/video"
)

func main() {
	// catch CTRL-C and clean up
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Println("caught CTRL-C")
		log.Printf("currently playing: %s", video.CurrentlyPlaying())
		events.LogoutAll(tripbot.Uptime)
		database.DBCon.Close()
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
