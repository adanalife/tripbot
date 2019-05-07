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

	client.OnPrivateMessage(func(message twitch.PrivateMessage) {
		log.Println("pm", message.User.Name, message.Message)
		if strings.Contains(strings.ToLower(message.Message), "ping") {
			client.Whisper(message.User.Name, "PONG")
			log.Println(message.User.Name, "PONG", message.Message)
		}
	})

	client.OnWhisperMessage(func(message twitch.WhisperMessage) {
		log.Println("whisp", message.User.Name, message.Message)
		if strings.Contains(strings.ToLower(message.Message), "ping") {
			client.Whisper(message.User.Name, "PONG")
			log.Println(message.User.Name, "PONG", message.Message)
		}
	})

	client.OnUserJoinMessage(func(joinMessage twitch.UserJoinMessage) {
		log.Println(joinMessage.Raw)
	})

	client.Join(channelToJoin)
	log.Println("Joined channel ", channelToJoin)

	err := client.Connect()
	if err != nil {
		panic(err)
	}
}
