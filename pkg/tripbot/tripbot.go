package tripbot

import (
	"fmt"
	"log"

	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/events"
	"github.com/gempir/go-twitch-irc/v2"
)

//TODO: can we make this private?
var Client *twitch.Client

func Initialize(username, token string) *twitch.Client {
	return twitch.NewClient(username, token)
}

func UserJoin(joinMessage twitch.UserJoinMessage) {
	events.LoginIfNecessary(joinMessage.User)
}

func UserPart(partMessage twitch.UserPartMessage) {
	events.LogoutIfNecessary(partMessage.User)
}

func UserNotice(message twitch.UserNoticeMessage) {
	log.Println("user notice:", message.SystemMsg, "***", message.Emotes, "***", message.Tags)
	// send message to chat if someone subs
	msg := fmt.Sprintf("%s Your support powers me bleedPurple", message.Message)
	Client.Say(config.ChannelName, msg)
}

func Whisper(message twitch.WhisperMessage) {
	log.Println("whisper from", message.User.Name, ":", message.Message)
	// if the message comes from me, then post the message to chat
	if message.User.Name == config.ChannelName {
		Client.Say(config.ChannelName, message.Message)
	}
}
