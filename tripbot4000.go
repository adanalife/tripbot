package main

import (
	"log"
	"os"
	"strings"

	twitch "github.com/gempir/go-twitch-irc"
)

const (
	clientUsername = "TripBot4000"
	channelToJoin  = "adanalife_"
)

func main() {
	clientAuthenticationToken, ok := os.LookupEnv("TWITCH_AUTH_TOKEN")
	if !ok {
		panic("You must set TWITCH_AUTH_TOKEN")
	}

	client := twitch.NewClient(clientUsername, clientAuthenticationToken)

	client.OnUserJoinMessage(func(joinMessage twitch.UserJoinMessage) {
		log.Println(joinMessage.Raw)
	})

	client.OnUserPartMessage(func(partMessage twitch.UserPartMessage) {
		log.Println(partMessage.Raw)
	})

	client.Join(channelToJoin)
	log.Println("Joined channel", channelToJoin)

	err := client.Connect()
	if err != nil {
		panic(err)
	}
}
