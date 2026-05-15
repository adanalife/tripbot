package chatbot

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	mylog "github.com/adanalife/tripbot/pkg/chatbot/log"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
	"github.com/adanalife/tripbot/pkg/users"
	"github.com/adanalife/tripbot/pkg/video"
	"github.com/davecgh/go-spew/spew"
	"github.com/gempir/go-twitch-irc/v4"
	"github.com/kelvins/geocoder"
	"github.com/nicklaw5/helix/v2"
)

var googleMapsAPIKey string
var client *twitch.Client
var Uptime time.Time

// App holds injectable dependencies for the chatbot.
// Tests instantiate it directly with fakes; production uses defaultApp.
type App struct {
	CurrentVideo func() video.Video
}

var defaultApp = &App{
	CurrentVideo: func() video.Video { return video.CurrentlyPlaying },
}

// used to determine which help message to display
// randomized so it starts with a new one every restart
var helpIndex = rand.Intn(len(c.HelpMessages))

const followerMsg = "Right now only followers of the channel can run unlimited commands :)"
const subscriberMsg = "You must be a subscriber to run that command :)"

// Initialize returns a Twitch client struct with all of the various configuration in place.
func Initialize() *twitch.Client {
	var err error
	Uptime = time.Now()

	// set up geocoder (for translating coords to places)
	geocoder.ApiKey = c.Conf.GoogleMapsAPIKey

	// initialize the twitch API client
	_, err = mytwitch.Client()
	if err != nil {
		terrors.Fatal(err, "unable to create twitch API client")
	}

	// The IRC token comes from the DB-backed oauth_tokens row populated by
	// cmd/auth-bootstrap; cmd/tripbot calls mytwitch.LoadFromDB before this.
	client = twitch.NewClient(c.Conf.BotUsername, mytwitch.IRCAuthToken())

	// attach handlers
	client.OnUserJoinMessage(UserJoin)
	client.OnUserPartMessage(UserPart)
	// client.OnUserNoticeMessage(chatbot.UserNotice)
	client.OnWhisperMessage(GetWhisper)
	client.OnPrivateMessage(PrivateMessage)

	return client
}

// Say will make a post in chat
func Say(msg string) {
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

// sayFn is the internal send implementation; tests override it to capture output.
var sayFn func(string) = Say

// Whisper will whisper a message to a user
// Note: go-twitch-irc v4 removed the Whisper() send method; we replicate the
// v2 behavior by sending the raw IRC /w command via PRIVMSG on the bot's own channel.
func Whisper(username, msg string) {
	//TODO: include whispers in log
	// include the message in the log
	// mylog.ChatMsg(c.Conf.BotUsername, msg)
	log.Println("sending whisper to", username, ":", msg)
	// say the message to chat
	client.Say(c.Conf.BotUsername, fmt.Sprintf("/w %s %s", username, msg))
}

// Chatter is designed to post a randomized message on a timer.
// Right now it just posts random "help messages."
func Chatter() {
	// use twitch emote feature to add some color
	sayFn("/me " + help())
}

func help() string {
	text := c.HelpMessages[helpIndex]
	// bump the index
	helpIndex = (helpIndex + 1) % len(c.HelpMessages)
	return text
}

// AnnounceNewFollower makes a post in chat that a user follows the channel
func AnnounceNewFollower(username string) {
	msg := fmt.Sprintf("Thank you for the follow, @%s", username)
	sayFn(msg)
}

// AnnounceSubscriber makes a post in chat that a user has subscribed
func AnnounceSubscriber(sub helix.Subscription) {
	//TODO: do more with the Subscription... IsGift, Tier, PlanName, etc.
	spew.Dump(sub)
	username := sub.UserName
	msg := fmt.Sprintf("Thank you for the sub, @%s; enjoy your !bonusmiles bleedPurple", username)
	sayFn(msg)
	// give everyone a bonus mile
	users.GiveEveryoneMiles(1.0)
	msg = fmt.Sprintf("The %d current viewers have been given a bonus mile, too HolidayPresent", len(users.LoggedIn))
	sayFn(msg)
}
