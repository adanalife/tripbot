package chatbot

import (
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/config"
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

	client = twitch.NewClient(botUsername, mytwitch.AuthToken)

	return client
}

// Chatter will post a message to chat
func Chatter() {
	// rand.Intn(len(config.HelpMessages))
	client.Say(config.ChannelName, help())
}

func help() string {
	text := config.HelpMessages[helpIndex]
	// bump the index
	helpIndex = (helpIndex + 1) % len(config.HelpMessages)
	return text
}

func AnnounceNewFollower(username string) {
	msg := fmt.Sprintf("Thank you for the follow, @%s", username)
	client.Say(config.ChannelName, msg)
}

//TODO: do more with the Subscription... IsGift, Tier, PlanName, etc.
func AnnounceSubscriber(sub helix.Subscription) {
	spew.Dump(sub)
	username := sub.UserName
	msg := fmt.Sprintf("Thank you for the sub, @%s; enjoy your !bonusmiles bleedPurple", username)
	client.Say(config.ChannelName, msg)
	// give everyone a bonus mile
	users.GiveEveryoneMiles(1.0)
	msg = fmt.Sprintf("The %d current viewers have been given a bonus mile, too HolidayPresent", len(users.LoggedIn))
	client.Say(config.ChannelName, msg)
}

