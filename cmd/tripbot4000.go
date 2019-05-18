package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/ocr"
	"github.com/dmerrick/danalol-stream/pkg/store"
	twitch "github.com/gempir/go-twitch-irc"
)

// used to determine which help message to display
var helpIndex = 0

// the most-recently processed video
var lastVid string

func main() {
	// first we must check for required ENV vars
	clientAuthenticationToken, ok := os.LookupEnv("TWITCH_AUTH_TOKEN")
	if !ok {
		panic("You must set TWITCH_AUTH_TOKEN")
	}

	// initialize the database
	//TODO: empty string is confusing
	datastore := store.FindOrCreate(helpers.DbPath)

	// time to set up the Twitch client
	client := twitch.NewClient(helpers.BotUsername, clientAuthenticationToken)

	client.OnUserJoinMessage(func(joinMessage twitch.UserJoinMessage) {
		if !helpers.UserIsIgnored(joinMessage.User) {
			datastore.RecordUserJoin(joinMessage.User)
		}
	})

	client.OnUserPartMessage(func(partMessage twitch.UserPartMessage) {
		if !helpers.UserIsIgnored(partMessage.User) {
			datastore.RecordUserPart(partMessage.User)
		}
	})

	client.OnWhisperMessage(func(message twitch.WhisperMessage) {
		log.Println("whisper:", message.User.Name, message.Message)
	})

	// all chat messages
	client.OnPrivateMessage(func(message twitch.PrivateMessage) {

		if strings.HasPrefix(strings.ToLower(message.Message), "!help") {
			log.Println(message.User.Name, "ran !help")
			msg := fmt.Sprintf("%s (run !help again for more)", helpers.HelpMessages[helpIndex])
			client.Say(helpers.ChannelName, msg)
			// bump the index
			helpIndex = (helpIndex + 1) % len(helpers.HelpMessages)
		}

		if strings.HasPrefix(strings.ToLower(message.Message), "!miles") {
			user := message.User.Name
			log.Println(user, "ran !miles")
			miles := datastore.MilesForUser(user)
			msg := ""
			switch {
			case miles == 1:
				msg = "@%s has only %d mile"
			case miles >= 250:
				msg = "Holy crap! @%s has %d miles!"
			default:
				msg = "@%s has %d miles. Earn 1 mile every 10 minutes by watching the stream"
			}
			msg = fmt.Sprintf(msg, user, miles)
			client.Say(helpers.ChannelName, msg)
		}

		if strings.HasPrefix(strings.ToLower(message.Message), "!tripbot") {
			log.Println(message.User.Name, "ran !tripbot")

			// get the currently-playing video
			currentVid := ocr.GetCurrentVideo()

			// only run once every 3 minutes
			if currentVid != lastVid {
				screenshotPath := ocr.ScreenshotPath(currentVid)
				// extract the coordinates, generate a google maps url
				url, err := ocr.ProcessImage(screenshotPath)
				if err != nil {
					client.Say(helpers.ChannelName, "Sorry, it didn't work this time :(. Try again in a few minutes!")
				} else {
					client.Say(helpers.ChannelName, fmt.Sprintf("If this doesn't work, try again in a few minutes: %s", url))
				}
				// update the last vid
				lastVid = currentVid
			} else {
				client.Say(helpers.ChannelName, fmt.Sprintf("I still need a minute, sorry!"))
			}

		}
	})

	// join the channel
	client.Join(helpers.ChannelName)
	log.Println("Joined channel", helpers.ChannelName)

	// actually connect to Twitch
	err := client.Connect()
	if err != nil {
		panic(err)
	}
}
