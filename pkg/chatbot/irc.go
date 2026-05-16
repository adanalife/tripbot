package chatbot

import (
	"fmt"
	"log"

	mylog "github.com/adanalife/tripbot/pkg/chatbot/log"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
)

// IRC is the subset of the IRC client surface chatbot commands depend on
// to send chat output. Tests inject a fake; production uses the package-backed
// realIRC adapter wired in defaultApp. Mirrors the Onscreens/VLC pattern.
type IRC interface {
	Say(msg string)               // post a message in chat
	Whisper(username, msg string) // whisper to a specific user
}

// realIRC delegates to the package-level twitch IRC client.
// Mirrors the existing Say()/Whisper() free functions in chatbot.go.
type realIRC struct{}

func (realIRC) Say(msg string) {
	// include the message in the log
	mylog.ChatMsg(c.Conf.BotUsername, msg)
	// figure out what channel to speak to
	speakTo := c.Conf.ChannelName
	if c.Conf.OutputChannel != "" {
		speakTo = c.Conf.OutputChannel
	}
	// say the message to chat
	client.Say(speakTo, msg)
}

func (realIRC) Whisper(username, msg string) {
	//TODO: include whispers in log
	log.Println("sending whisper to", username, ":", msg)
	// go-twitch-irc v4 removed the Whisper() send method; replicate the
	// v2 behavior by sending the raw IRC /w command via PRIVMSG on the
	// bot's own channel.
	client.Say(c.Conf.BotUsername, fmt.Sprintf("/w %s %s", username, msg))
}
