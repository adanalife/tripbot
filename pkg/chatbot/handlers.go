package chatbot

import (
	"fmt"
	"log"
	"strings"

	mylog "github.com/adanalife/tripbot/pkg/chatbot/log"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/adanalife/tripbot/pkg/users"
	"github.com/gempir/go-twitch-irc/v2"
)

func incChatCommandCounter(command string) {
	if cnt, err := instrumentation.ChatCommands.GetMetricWithLabelValues(command); err == nil {
		cnt.Add(1)
	}
}

func runCommand(user *users.User, message string) {
	var err error
	var params []string

	msg := strings.TrimSpace(message)
	split := strings.Split(msg, " ")

	// the command is the first part
	command := split[0]

	if len(split) > 1 {
		// the params are the second part
		params = split[1:]

		// this invalid unicode character shows up when you run the same command twice
		// (it may be specific to Chatterino as a twitch client?)
		if params[len(params)-1] == "\U000e0000" {
			params = params[:len(params)-1]
		}
	}

	// handle case where people add a space (like "! location")
	if command == "!" {
		command = command + params[0]
		// remove the first element from the params
		params = params[1:]
	}

	switch command {
	case "!help":
		incChatCommandCounter("!help")
		helpCmd(user)
	case "hello", "hi", "hey", "hallo", "!bot":
		incChatCommandCounter("hello")
		helloCmd(user, params)
	case "!flag":
		incChatCommandCounter("!flag")
		flagCmd(user)
	case "!version":
		incChatCommandCounter("!version")
		versionCmd(user)
	case "!uptime":
		uptimeCmd(user)
	case "!timewarp", "!timewrap", "!timeskip", "!tw", "!timewqrp", "!warp":
		if user.HasCommandAvailable() {
			timewarpCmd(user)
		} else {
			Say(followerMsg)
		}
	case "!goto", "!jump":
		if user.HasCommandAvailable() {
			jumpCmd(user, params)
		} else {
			Say(followerMsg)
		}
	case "!skip":
		if user.HasCommandAvailable() {
			skipCmd(user, params)
		} else {
			Say(followerMsg)
		}
	case "!back":
		if user.HasCommandAvailable() {
			backCmd(user, params)
		} else {
			Say(followerMsg)
		}
	case "!shutdown":
		shutdownCmd(user)
	case "!socialmedia", "!social", "!socials":
		Say("Find me outside of Twitch: !twitter, !instagram, !facebook, !youtube")
	case "!commands", "!command", "¡command", "¡commands", "!commads", "!controls", "!commande":
		Say("You can try: !location, !guess, !date, !state, !sunset, !timewarp, !miles, !leaderboard, and many other hidden commands!")
	case "!bonusmiles":
		if user.IsSubscriber() {
			bonusMilesCmd(user)
		} else {
			Say(subscriberMsg)
		}
	case "!sunset", "!sunet":
		if user.HasCommandAvailable() {
			sunsetCmd(user)
		} else {
			Say(followerMsg)
		}
	case "!time", "!timr":
		if user.HasCommandAvailable() {
			timeCmd(user)
		} else {
			Say(followerMsg)
		}
	case "!date", "!datw":
		if user.HasCommandAvailable() {
			dateCmd(user)
		} else {
			Say(followerMsg)
		}
	case "!guess", "!guss", "guess", "!gusss", "!guees", "!gues", "!quess", "!guis":
		if user.HasCommandAvailable() {
			guessCmd(user, params)
		} else {
			Say(followerMsg)
		}
	case "!state":
		if user.HasCommandAvailable() {
			stateCmd(user)
		} else {
			Say(followerMsg)
		}
	case "!secretinfo":
		secretInfoCmd(user)
	case "!gas", "!fuel", "!petrol":
		Say("About full, thanks for asking")
	case "!middle":
		middleCmd(user, params)
		// any of these should trigger the miles command
	case "!miles", "!points":
		if user.HasCommandAvailable() {
			milesCmd(user, params)
		} else {
			Say(followerMsg)
		}

		// any of these should trigger the kilometres command
	case "!km", "!kilometres", "!kilometers":
		if user.HasCommandAvailable() {
			kilometresCmd(user)
		} else {
			Say(followerMsg)
		}

		// any of these should trigger the location command
		//TODO: add support for: "where is this", "where are we", "where are you"
	case "!tripbot", "!location", "!city", "!town", "!where", "!loacation", "!loation", "!loc", "!locatioin", "!locatoion", "!locaton", "!loclistion", "!locton", "1location", "¡location", "!locatiom", "!location!", "!locatio":
		if user.HasCommandAvailable() {
			locationCmd(user)
		} else {
			Say(followerMsg)
		}

		// trigger the leaderboard command
	case "!leaderboard", "!monthlyleaderboard", "!lb", "!mlb", "!leaderbord", "!ldb", "!ldbd":
		if user.HasCommandAvailable() {
			monthlyMilesLeaderboardCmd(user)
		} else {
			Say(followerMsg)
		}

		// trigger the lifetime leaderboard command
	case "!totalleaderboard", "!lifetimeleaderboard", "!tlb", "!llb":
		if user.HasCommandAvailable() {
			lifetimeMilesLeaderboardCmd(user)
		} else {
			Say(followerMsg)
		}

		// trigger the lifetime leaderboard command
	case "!guessleaderboard", "!glb":
		if user.HasCommandAvailable() {
			monthlyGuessLeaderboardCmd(user)
		} else {
			Say(followerMsg)
		}

		// any of these should trigger the report command
		//TODO: probably want to allow people to run this more than once?
		//TODO: the two-word ones dont work
	case "!report", "no audio", "no sound", "no music", "frozen":
		if user.HasCommandAvailable() {
			reportCmd(user, params)
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
	instrumentation.ChatMessages.Inc()

	//TODO: we lose capitalization here, is that okay?
	message := strings.ToLower(msg.Message)

	// log to stackdriver
	mylog.ChatMsg(username, msg.Message)

	// check to see if the message is a command
	//TODO: also include ones prefixed with whitespace?
	// log in the user
	user := users.LoginIfNecessary(username)

	runCommand(user, message)
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
func GetWhisper(message twitch.WhisperMessage) {
	log.Println("whisper from", message.User.Name, ":", message.Message)
	if c.UserIsAdmin(message.User.Name) {
		Say(message.Message)
	}
}
