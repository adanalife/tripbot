package tripbot

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/database"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	mytwitch "github.com/dmerrick/danalol-stream/pkg/twitch"
	"github.com/dmerrick/danalol-stream/pkg/users"
	"github.com/gempir/go-twitch-irc/v2"
	"github.com/joho/godotenv"
	"github.com/kelvins/geocoder"
	"github.com/logrusorgru/aurora"
)

var botUsername, googleMapsAPIKey string
var client *twitch.Client
var Uptime time.Time

// used to determine which help message to display
// randomized so it starts with a new one every restart
var helpIndex = rand.Intn(len(config.HelpMessages))

const followerMsg = "Follow the stream to run unlimited commands :)"
const subscriberMsg = "You must be a subscriber to run that command :)"

// all chat messages
func PrivateMessage(message twitch.PrivateMessage) {
	username := message.User.Name

	// log in the user
	user := users.LoginIfNecessary(username)

	if strings.HasPrefix(strings.ToLower(message.Message), "!help") {
		helpCmd(user)
	}

	if strings.HasPrefix(strings.ToLower(message.Message), "!uptime") {
		uptimeCmd(user)
	}

	//TODO: rename this to oldmiles
	if strings.HasPrefix(strings.ToLower(message.Message), "!miles") {
		if user.HasCommandAvailable() {
			oldMilesCmd(user)
		} else {
			client.Say(config.ChannelName, followerMsg)
		}
	}

	//TODO: rename this to miles
	if strings.HasPrefix(strings.ToLower(message.Message), "!newmiles") {
		if user.HasCommandAvailable() {
			milesCmd(user)
		} else {
			client.Say(config.ChannelName, followerMsg)
		}
	}

	if strings.HasPrefix(strings.ToLower(message.Message), "!sunset") {
		if user.HasCommandAvailable() {
			sunsetCmd(user)
		} else {
			client.Say(config.ChannelName, followerMsg)
		}
	}

	// any of these should trigger the location command
	locationStrings := []string{
		"!tripbot",
		"!location",
		"!locton",
		"!locaton",
		"!locatoion",
		"where is this",
		"where are we",
		"where are you",
	}
	for _, s := range locationStrings {
		if strings.HasPrefix(strings.ToLower(message.Message), s) {
			if user.HasCommandAvailable() {
				locationCmd(user)
			} else {
				client.Say(config.ChannelName, followerMsg)
			}
		}
	}

	if strings.HasPrefix(strings.ToLower(message.Message), "!leaderboard") {
		if user.HasCommandAvailable() {
			oldLeaderboardCmd(user)
		} else {
			client.Say(config.ChannelName, followerMsg)
		}
	}

	if strings.HasPrefix(strings.ToLower(message.Message), "!newleaderboard") {
		if user.HasCommandAvailable() {
			leaderboardCmd(user)
		} else {
			client.Say(config.ChannelName, followerMsg)
		}
	}

	if strings.HasPrefix(strings.ToLower(message.Message), "!time") {
		if user.HasCommandAvailable() {
			timeCmd(user)
		} else {
			client.Say(config.ChannelName, followerMsg)
		}
	}

	if strings.HasPrefix(strings.ToLower(message.Message), "!date") {
		if user.HasCommandAvailable() {
			dateCmd(user)
		} else {
			client.Say(config.ChannelName, followerMsg)
		}
	}

	if strings.HasPrefix(strings.ToLower(message.Message), "!guess") {
		if user.HasCommandAvailable() {
			guessCmd(user, message.Message)
		} else {
			client.Say(config.ChannelName, followerMsg)
		}
	}

	if strings.HasPrefix(strings.ToLower(message.Message), "!state") {
		if user.HasCommandAvailable() {
			stateCmd(user)
		} else {
			client.Say(config.ChannelName, followerMsg)
		}
	}

	if strings.HasPrefix(strings.ToLower(message.Message), "!secretinfo") {
		secretInfoCmd(user)
	}

	// any of these should trigger the report command
	reportStrings := []string{
		"!report",
		"no audio",
		"no sound",
		"no music",
		"frozen",
	}
	for _, rs := range reportStrings {
		if strings.HasPrefix(strings.ToLower(message.Message), rs) {
			if user.HasCommandAvailable() {
				reportCmd(user, message.Message)
			} else {
				client.Say(config.ChannelName, followerMsg)
			}
		}
	}

	if strings.HasPrefix(strings.ToLower(message.Message), "!shutdown") {
		shutdownCmd(user)
	}

	if strings.HasPrefix(strings.ToLower(message.Message), "!onlyatest") {
		if user.IsSubscriber() {
			client.Say(config.ChannelName, "It worked!")
		} else {
			client.Say(config.ChannelName, subscriberMsg)
		}
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
func UserNotice(message twitch.UserNoticeMessage) {
	// update the internal subscriber list
	mytwitch.GetSubscribers()

	if message.Message != "" {
		msg := fmt.Sprintf("%s just subscribed with message: %s", message.User.DisplayName, message.Message)
		client.Say(config.ChannelName, msg)
	} else {
		msg := fmt.Sprintf("%s just subscribed", message.User.DisplayName)
		client.Say(config.ChannelName, msg)
	}
	client.Say(config.ChannelName, "Thank you. Your support keeps me running bleedPurple")
}

// if the message comes from me, then post the message to chat
func Whisper(message twitch.WhisperMessage) {
	log.Println("whisper from", message.User.Name, ":", message.Message)
	if message.User.Name == strings.ToLower(config.ChannelName) {
		client.Say(config.ChannelName, message.Message)
	}
}

func Initialize() *twitch.Client {
	var err error
	Uptime = time.Now()

	// load ENV vars from .env file
	err = godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// first we must check for required ENV vars
	if os.Getenv("DASHCAM_DIR") == "" {
		panic("You must set DASHCAM_DIR")
	}
	botUsername = os.Getenv("BOT_USERNAME")
	if botUsername == "" {
		panic("You must set BOT_USERNAME")
	}
	googleMapsAPIKey = os.Getenv("GOOGLE_MAPS_API_KEY")
	if googleMapsAPIKey == "" {
		panic("You must set GOOGLE_MAPS_API_KEY")
	}

	// set up geocoder (for translating coords to places)
	geocoder.ApiKey = googleMapsAPIKey

	// initialize the twitch API client
	c, err := mytwitch.Client()
	if err != nil {
		terrors.Fatal(err, "unable to create twitch API client")
	}

	//TODO: actually use these security features
	authURL := c.GetAuthorizationURL("", false)
	log.Println("if your browser doesn't open automatically:")
	log.Println(aurora.Blue(authURL).Underline())
	helpers.OpenInBrowser(authURL)

	// initialize the SQL database
	database.DBCon, err = database.Initialize()
	if err != nil {
		terrors.Fatal(err, "error initializing the DB")
	}

	client = twitch.NewClient(botUsername, mytwitch.AuthToken)

	return client
}
