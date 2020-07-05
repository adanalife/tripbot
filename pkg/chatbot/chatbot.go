package chatbot

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/adanalife/tripbot/pkg/background"
	"github.com/adanalife/tripbot/pkg/config"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
	mylog "github.com/adanalife/tripbot/pkg/log"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
	"github.com/adanalife/tripbot/pkg/users"
	"github.com/gempir/go-twitch-irc/v2"
	"github.com/kelvins/geocoder"
	"github.com/logrusorgru/aurora"
	"github.com/nicklaw5/helix"
)

var googleMapsAPIKey string
var client *twitch.Client
var Uptime time.Time

// used to determine which help message to display
// randomized so it starts with a new one every restart
var helpIndex = rand.Intn(len(config.HelpMessages))

const followerMsg = "Follow the stream to run unlimited commands :)"
const subscriberMsg = "You must be a subscriber to run that command :)"

func Initialize() *twitch.Client {
	var err error
	Uptime = time.Now()

	// set up geocoder (for translating coords to places)
	geocoder.ApiKey = config.GoogleMapsAPIKey

	// initialize the twitch API client
	c, err := mytwitch.Client()
	if err != nil {
		terrors.Fatal(err, "unable to create twitch API client")
	}

	if !config.DisableTwitchWebhooks {
		//TODO: actually use these security features
		authURL := c.GetAuthorizationURL("", false)
		log.Println("if your browser doesn't open automatically:")
		log.Println(aurora.Blue(authURL).Underline())
		helpers.OpenInBrowser(authURL)
	}

	client = twitch.NewClient(config.BotUsername, mytwitch.AuthToken)

	// attach handlers
	client.OnUserJoinMessage(UserJoin)
	client.OnUserPartMessage(UserPart)
	// client.OnUserNoticeMessage(chatbot.UserNotice)
	client.OnWhisperMessage(Whisper)
	client.OnPrivateMessage(PrivateMessage)

	return client
}

// Say will make a post in chat
func Say(msg string) {
	// include the message in the log
	mylog.ChatMsg(config.BotUsername, msg)
	// include the bot output in chat
	background.AddChatLine(config.BotUsername, msg)
	// say the message to chat
	client.Say(config.ChannelName, msg)
}

// Chatter is designed to most a randomized message on a timer
// right now it just posts random "help messages"
func Chatter() {
	// rand.Intn(len(config.HelpMessages))
	// use twitch emote feature to add some color
	Say("/me " + help())
}

func help() string {
	text := config.HelpMessages[helpIndex]
	// bump the index
	helpIndex = (helpIndex + 1) % len(config.HelpMessages)
	return text
}

func AnnounceNewFollower(username string) {
	msg := fmt.Sprintf("Thank you for the follow, @%s", username)
	Say(msg)
}

// AnnounceSubscriber makes a post in chat that a user has subscribed
func AnnounceSubscriber(sub helix.Subscription) {
	//TODO: do more with the Subscription... IsGift, Tier, PlanName, etc.
	spew.Dump(sub)
	username := sub.UserName
	msg := fmt.Sprintf("Thank you for the sub, @%s; enjoy your !bonusmiles bleedPurple", username)
	Say(msg)
	// give everyone a bonus mile
	users.GiveEveryoneMiles(1.0)
	msg = fmt.Sprintf("The %d current viewers have been given a bonus mile, too HolidayPresent", len(users.LoggedIn))
	Say(msg)
}
