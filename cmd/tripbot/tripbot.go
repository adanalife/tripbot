package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/dimiro1/banner/autoload"
	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/events"
	"github.com/dmerrick/danalol-stream/pkg/tripbot"
)

func main() {
	// catch CTRL-C and clean up
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Println("caught CTRL-C")
		events.LogoutAll(uptime)
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
	err = client.Connect()
	if err != nil {
		panic(err)
	}
}
