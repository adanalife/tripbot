package tripbot

import (
	"github.com/dmerrick/danalol-stream/pkg/events"
	"github.com/gempir/go-twitch-irc"
)

func UserJoin(joinMessage twitch.UserJoinMessage) {
	events.LoginIfNecessary(joinMessage.User)
}
