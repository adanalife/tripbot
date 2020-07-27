package chatbot

import (
	"fmt"
	"log"
	"strings"

	"github.com/adanalife/tripbot/pkg/background"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
	mylog "github.com/adanalife/tripbot/pkg/log"
	"github.com/adanalife/tripbot/pkg/users"
	"github.com/gempir/go-twitch-irc/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	chatMessages = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tripbot_chat_messages_total",
		Help: "The total number of chat messages",
	})
	chatCommands = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tripbot_chat_commands_total",
		Help: "The total number of chat commands",
	}, []string{"command"},
	)
)

func incChatCommandCounter(command string) {
	if cnt, err := chatCommands.GetMetricWithLabelValues(command); err == nil {
		cnt.Add(1)
	}
}

func runCommand(user users.User, message string) {
	var err error

	split := strings.Split(message, " ")
	command := split[0]
	params := split[1:]

	switch command {
	case "!help":
		incChatCommandCounter("!help")
		helpCmd(&user)
	case "hello", "hi", "hey", "hallo":
		incChatCommandCounter("hello")
		helloCmd(&user, params)
	case "!flag":
		incChatCommandCounter("!flag")
		flagCmd(&user)
	case "!version":
		incChatCommandCounter("!version")
		versionCmd(&user)
	case "!song", "!currentsong", "!music", "!currentmusic":
		songCmd(&user)
	case "!uptime":
		uptimeCmd(&user)
		//TODO: remove this
	case "!oldmiles":
		if user.HasCommandAvailable() {
			oldMilesCmd(&user)
		} else {
			Say(followerMsg)
		}
	case "!timewarp", "!tw":
		if user.HasCommandAvailable() {
			timewarpCmd(&user)
		} else {
			Say(followerMsg)
		}
	case "!goto", "!jump":
		if user.HasCommandAvailable() {
			jumpCmd(&user, params)
		} else {
			Say(followerMsg)
		}
	case "!skip":
		if user.HasCommandAvailable() {
			skipCmd(&user, params)
		} else {
			Say(followerMsg)
		}
	case "!back":
		if user.HasCommandAvailable() {
			backCmd(&user, params)
		} else {
			Say(followerMsg)
		}
	case "!shutdown":
		shutdownCmd(&user)
	case "!restartmusic":
		restartMusicCmd(&user)
	case "!socialmedia":
		Say("Find me outside of Twitch: !twitter, !instagram, !facebook, !youtube")
	case "!commands", "!controls":
		Say("You can try: !location, !guess, !date, !state, !sunset, !timewarp, !miles, !leaderboard, and many other hidden commands!")
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
	case "!middle":
		middleCmd(&user, params)
		// any of these should trigger the miles command
	case "!miles", "!points":
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
		//TODO: add support for: "where is this", "where are we", "where are you"
	case "!tripbot", "!location", "!locton", "!locaton", "!locatoion", "1location", "!city", "!town":
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
		if strings.HasPrefix(command, "!") {
			// log the command as an error so we can implement it in the future
			err = fmt.Errorf("command %s not found", command)
		}
	}
	if err != nil {
		terrors.Log(err, "error running command")
	}
}

// handles all chat messages
func PrivateMessage(msg twitch.PrivateMessage) {
	username := msg.User.Name

	// increment the Prometheus counter
	chatMessages.Inc()

	//TODO: we lose capitalization here, is that okay?
	message := strings.ToLower(msg.Message)

	// log to stackdriver
	mylog.ChatMsg(username, msg.Message)

	// include in the onscreen chat box
	background.AddChatLine(username, msg.Message)

	// check to see if the message is a command
	//TODO: also include ones prefixed with whitespace?
	// log in the user
	user := users.LoginIfNecessary(username)

	runCommand(*user, message)
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
	if helpers.UserIsAdmin(message.User.Name) {
		Say(message.Message)
	}
}
