package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/ocr"
	"github.com/dmerrick/danalol-stream/pkg/store"
	mytwitch "github.com/dmerrick/danalol-stream/pkg/twitch"
	twitch "github.com/gempir/go-twitch-irc"
	"github.com/kelvins/geocoder"
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
	twitchClientID, ok := os.LookupEnv("TWITCH_CLIENT_ID")
	if !ok {
		panic("You must set TWITCH_CLIENT_ID")
	}
	googleMapsAPIKey, ok := os.LookupEnv("GOOGLE_MAPS_API_KEY")
	if !ok {
		panic("You must set GOOGLE_MAPS_API_KEY")
	}
	geocoder.ApiKey = googleMapsAPIKey

	// initialize the twitch API client
	_, err := mytwitch.FindOrCreateClient(twitchClientID)
	if err != nil {
		log.Fatal("unable to create twitch API client", err)
	}

	// initialize the database
	datastore := store.FindOrCreate(config.DbPath)

	// time to set up the Twitch client
	client := twitch.NewClient(config.BotUsername, clientAuthenticationToken)

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

	client.OnUserNoticeMessage(func(message twitch.UserNoticeMessage) {
		log.Println("user notice:", message.User, message.Type, message.Message, message.SystemMsg, message.Emotes, message.Tags)
	})

	client.OnWhisperMessage(func(message twitch.WhisperMessage) {
		log.Println("whisper:", message.User.Name, message.Message)
		// if the message comes from me, then post the message to chat
		if message.User.Name == config.ChannelName {
			client.Say(config.ChannelName, message.Message)
		}
	})

	// all chat messages
	client.OnPrivateMessage(func(message twitch.PrivateMessage) {
		//TODO: use this
		user := message.User.Name

		// just print if the user is a follower for now
		//TODO error handling
		isFollower, _ := mytwitch.UserIsFollower(user)
		log.Println(user, "is follower?", isFollower)

		if strings.HasPrefix(strings.ToLower(message.Message), "!help") {
			log.Println(message.User.Name, "ran !help")
			msg := fmt.Sprintf("%s (repeat this command for more)", config.HelpMessages[helpIndex])
			client.Say(config.ChannelName, msg)
			// bump the index
			helpIndex = (helpIndex + 1) % len(config.HelpMessages)
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
			client.Say(config.ChannelName, msg)
		}

		if strings.HasPrefix(strings.ToLower(message.Message), "!tripbot") {
			log.Println(message.User.Name, "ran !tripbot")

			// get the currently-playing video
			currentVid := ocr.GetCurrentVideo()

			// only run if this video hasn't yet been processed
			if currentVid != lastVid {
				// extract the coordinates
				lat, lon, err := datastore.CoordsFromVideoPath(currentVid)
				if err != nil {
					client.Say(config.ChannelName, "Sorry, it didn't work this time :(. Try again in a few minutes!")
				} else {
					// generate a google maps url
					address, _ := helpers.CityFromCoords(lat, lon)
					if err != nil {
						log.Println("geocoding error", err)
					}
					url := helpers.GoogleMapsURL(lat, lon)
					msg := fmt.Sprintf("%s %s", address, url)
					client.Say(config.ChannelName, msg)
				}
				// update the last vid
				lastVid = currentVid
			} else {
				client.Say(config.ChannelName, fmt.Sprintf("That's too soon, I need a minute"))
			}

		}

		if strings.HasPrefix(strings.ToLower(message.Message), "!leaderboard") {
			log.Println(message.User.Name, "ran !leaderboard")
			userList := datastore.TopUsers(3)
			for i, user := range userList {
				msg := fmt.Sprintf("#%d: %s (%dmi)", i+1, user, datastore.MilesForUser(user))
				client.Say(config.ChannelName, msg)
			}
		}

		if strings.HasPrefix(strings.ToLower(message.Message), "!date") {
			log.Println(message.User.Name, "ran !date")
			// get the currently-playing video
			currentVid := ocr.GetCurrentVideo()
			lat, lon, err := datastore.CoordsFromVideoPath(currentVid)
			if err != nil {
				client.Say(config.ChannelName, "That didn't work, sorry!")
			} else {
				vidDate := helpers.VidStrToDate(currentVid)
				realDate := helpers.ActualDate(vidDate, lat, lon)
				// "Mon, 02 Jan 2006 15:04:05 MST"
				fmtDate := realDate.Format(time.RFC1123)
				client.Say(config.ChannelName, fmt.Sprintf("This moment was %s", fmtDate))
			}
		}

		if strings.HasPrefix(strings.ToLower(message.Message), "!state") {
			log.Println(message.User.Name, "ran !state")
			// get the currently-playing video
			currentVid := ocr.GetCurrentVideo()
			lat, lon, err := datastore.CoordsFromVideoPath(currentVid)
			if err != nil {
				client.Say(config.ChannelName, "That didn't work, sorry!")
			} else {
				state, _ := helpers.StateFromCoords(lat, lon)
				msg := fmt.Sprintf("We're in %s", state)
				client.Say(config.ChannelName, msg)
			}
		}
	})

	// join the channel
	client.Join(config.ChannelName)
	log.Println("Joined channel", config.ChannelName)

	// actually connect to Twitch
	err = client.Connect()
	if err != nil {
		panic(err)
	}
}
