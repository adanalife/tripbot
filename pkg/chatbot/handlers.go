package chatbot

import (
	"fmt"
	"log"
	"strings"

	"github.com/dmerrick/danalol-stream/pkg/background"
	"github.com/dmerrick/danalol-stream/pkg/config"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	mylog "github.com/dmerrick/danalol-stream/pkg/log"
	"github.com/dmerrick/danalol-stream/pkg/users"
	"github.com/gempir/go-twitch-irc/v2"
)

func runCommand(user users.User, message string) {
	var err error

	split := strings.Split(message, " ")
	command := split[0]
	params := split[1:]

	switch command {
	case "!help":
		helpCmd(&user)
	case "!song":
		songCmd(&user)
	case "!uptime":
		uptimeCmd(&user)
	case "!oldmiles":
		if user.HasCommandAvailable() {
			oldMilesCmd(&user)
		} else {
			Say(followerMsg)
		}
	case "!shutdown":
		shutdownCmd(&user)
	case "!restartmusic":
		restartMusicCmd(&user)
	case "!bonusmiles":
		if user.IsSubscriber() {
			bonusMilesCmd(&user)
		} else {
			Say(subscriberMsg)
		}
	case "!sunset":
		if user.HasCommandAvailable() {
			sunsetCmd(&user)
		} else {
			Say(followerMsg)
		}
	case "!oldleaderboard":
		if user.HasCommandAvailable() {
			oldLeaderboardCmd(&user)
		} else {
			Say(followerMsg)
		}
	case "!time":
		if user.HasCommandAvailable() {
			timeCmd(&user)
		} else {
			Say(followerMsg)
		}
	case "!date":
		if user.HasCommandAvailable() {
			dateCmd(&user)
		} else {
			Say(followerMsg)
		}
	case "!guess":
		if user.HasCommandAvailable() {
			guessCmd(&user, params)
		} else {
			Say(followerMsg)
		}
	case "!state":
		if user.HasCommandAvailable() {
			stateCmd(&user)
		} else {
			Say(followerMsg)
		}
	case "!secretinfo":
		secretInfoCmd(&user)
		// any of these should trigger the miles command
	case "!miles", "!newmiles":
		if user.HasCommandAvailable() {
			milesCmd(&user)
		} else {
			Say(followerMsg)
		}

		// any of these should trigger the kilometres command
	case "!km", "!kilometres", "!kilometers":
		if user.HasCommandAvailable() {
			kilometresCmd(&user)
		} else {
			Say(followerMsg)
		}

		// any of these should trigger the location command
	case "!tripbot", "!location", "!locton", "!locaton", "!locatoion", "where is this", "where are we", "where are you":
		if user.HasCommandAvailable() {
			locationCmd(&user)
		} else {
			Say(followerMsg)
		}

		// any of these should trigger the leaderboard command
	case "!leaderboard", "!newleaderboard":
		if user.HasCommandAvailable() {
			leaderboardCmd(&user)
		} else {
			Say(followerMsg)
		}

		// any of these should trigger the report command
		//TODO: probably want to allow people to run this more than once?
	case "!report", "no audio", "no sound", "no music", "frozen":
		if user.HasCommandAvailable() {
			reportCmd(&user, params)
		} else {
			Say(followerMsg)
		}
	default:
		err = fmt.Errorf("command %s not found", command)
	}
	if err != nil {
		terrors.Log(err, "error running command")
	}
}

// handles all chat messages
func PrivateMessage(msg twitch.PrivateMessage) {
	username := msg.User.Name
	//TODO: we lose capitalization here, is that okay?
	message := strings.ToLower(msg.Message)

	// log to stackdriver
	mylog.ChatMsg(username, msg.Message)

	// include in the onscreen chat box
	background.AddChatLine(username, msg.Message)

	// check to see if the message is a command
	//TODO: also include ones prefixed with whitespace?
	//TODO: not all commands start with "!"s
	if strings.HasPrefix(message, "!") {
		// log in the user
		user := users.LoginIfNecessary(username)

		//TODO is it okay that this isn't a pointer?
		runCommand(*user, message)
	}
}

// this event fires when a user joins the channel
func UserJoin(joinMessage twitch.UserJoinMessage) {
	users.LoginIfNecessary(joinMessage.User)
}

// this event fires when a user leaves the channel
func UserPart(partMessage twitch.UserPartMessage) {
	users.LogoutIfNecessary(partMessage.User)
}

// send message to chat if someone subs
// func UserNotice(message twitch.UserNoticeMessage) {
// 	// update the internal subscriber list
// 	mytwitch.GetSubscribers()
// }

// if the message comes from me, then post the message to chat
//TODO: log to stackdriver
func Whisper(message twitch.WhisperMessage) {
	log.Println("whisper from", message.User.Name, ":", message.Message)
	if message.User.Name == strings.ToLower(config.ChannelName) {
		Say(message.Message)
	}
}
