package main

import (
	"log"
	"os"

	twitch "github.com/gempir/go-twitch-irc"
)

const (
	clientUsername = "TripBot4000"
	channelToJoin  = "adanalife_"
)

var ignoredUsers = []string{
	"anotherttvviewer",
	"commanderroot",
	"electricallongboard",
	"logviewer",
}

func userIsIgnored(user string) bool {
	for _, ignored := range ignoredUsers {
		if user == ignored {
			return true
		}
	}
	return false
}

func main() {
	clientAuthenticationToken, ok := os.LookupEnv("TWITCH_AUTH_TOKEN")
	if !ok {
		panic("You must set TWITCH_AUTH_TOKEN")
	}

	client := twitch.NewClient(clientUsername, clientAuthenticationToken)

	client.OnUserJoinMessage(func(joinMessage twitch.UserJoinMessage) {
		if !userIsIgnored(joinMessage.User) {
			log.Println(joinMessage.Raw)
		}
	})

	client.OnUserPartMessage(func(partMessage twitch.UserPartMessage) {
		if !userIsIgnored(partMessage.User) {
			log.Println(partMessage.Raw)
		}
	})

	client.Join(channelToJoin)
	log.Println("Joined channel", channelToJoin)

	err := client.Connect()
	if err != nil {
		panic(err)
	}
}
