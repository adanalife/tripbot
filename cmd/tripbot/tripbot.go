package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/dimiro1/banner/autoload"
	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/database"
	"github.com/dmerrick/danalol-stream/pkg/events"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/miles"
	"github.com/dmerrick/danalol-stream/pkg/store"
	mytwitch "github.com/dmerrick/danalol-stream/pkg/twitch"
	"github.com/dmerrick/danalol-stream/pkg/video"
	twitch "github.com/gempir/go-twitch-irc"
	"github.com/hako/durafmt"
	"github.com/joho/godotenv"
	"github.com/kelvins/geocoder"
)

// used to determine which help message to display
// randomized so it starts with a new one every restart
var helpIndex = rand.Intn(len(config.HelpMessages))

// the most-recently processed video
var lastVid string

func main() {
	uptime := time.Now()

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// catch CTRL-C and clean up
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Println("caught CTRL-C")
		events.LogoutAll(uptime)
		os.Exit(1)
	}()

	// first we must check for required ENV vars
	if os.Getenv("DASHCAM_DIR") == "" {
		panic("You must set DASHCAM_DIR")
	}
	botUsername := os.Getenv("BOT_USERNAME")
	if botUsername == "" {
		panic("You must set BOT_USERNAME")
	}
	clientAuthenticationToken := os.Getenv("TWITCH_AUTH_TOKEN")
	if clientAuthenticationToken == "" {
		panic("You must set TWITCH_AUTH_TOKEN")
	}
	twitchClientID := os.Getenv("TWITCH_CLIENT_ID")
	if twitchClientID == "" {
		panic("You must set TWITCH_CLIENT_ID")
	}
	googleMapsAPIKey := os.Getenv("GOOGLE_MAPS_API_KEY")
	if googleMapsAPIKey == "" {
		panic("You must set GOOGLE_MAPS_API_KEY")
	}
	geocoder.ApiKey = googleMapsAPIKey

	// initialize the twitch API client
	_, err = mytwitch.FindOrCreateClient(twitchClientID)
	if err != nil {
		log.Fatal("unable to create twitch API client", err)
	}

	// initialize the SQL database
	database.DBCon, err = database.Initialize()
	if err != nil {
		log.Fatal("error initializing the DB", err)
	}
	defer database.DBCon.Close()

	// initialize the local datastore
	datastore := store.FindOrCreate(config.DBPath)
	// make sure everyone is logged out
	// datastore.ClearJoinBucket()

	// set up the Twitch client
	client := twitch.NewClient(botUsername, clientAuthenticationToken)

	client.OnUserJoinMessage(func(joinMessage twitch.UserJoinMessage) {
		events.LoginIfNecessary(joinMessage.User)
		if !helpers.UserIsIgnored(joinMessage.User) {
			datastore.RecordUserJoin(joinMessage.User)
		}
	})

	client.OnUserPartMessage(func(partMessage twitch.UserPartMessage) {
		events.LogoutIfNecessary(partMessage.User)
		if !helpers.UserIsIgnored(partMessage.User) {
			datastore.RecordUserPart(partMessage.User)
		}
	})

	client.OnUserNoticeMessage(func(message twitch.UserNoticeMessage) {
		log.Println("user notice:", message.SystemMsg, "***", message.Emotes, "***", message.Tags)
		// send message to chat if someone subs
		msg := fmt.Sprintf("%s Your support powers me bleedPurple", message.Message)
		client.Say(config.ChannelName, msg)
	})

	client.OnWhisperMessage(func(message twitch.WhisperMessage) {
		log.Println("whisper from", message.User.Name, ":", message.Message)
		// if the message comes from me, then post the message to chat
		if message.User.Name == config.ChannelName {
			client.Say(config.ChannelName, message.Message)
		}
	})

	// all chat messages
	client.OnPrivateMessage(func(message twitch.PrivateMessage) {
		user := message.User.Name

		// log the user in if their login time isn't currently recorded
		events.LoginIfNecessary(user)

		if strings.HasPrefix(strings.ToLower(message.Message), "!help") {
			log.Println(user, "ran !help")
			msg := fmt.Sprintf("%s (%d of %d)", config.HelpMessages[helpIndex], helpIndex+1, len(config.HelpMessages))
			client.Say(config.ChannelName, msg)
			// bump the index
			helpIndex = (helpIndex + 1) % len(config.HelpMessages)
		}

		if strings.HasPrefix(strings.ToLower(message.Message), "!uptime") {
			log.Println(user, "ran !uptime")
			dur := time.Now().Sub(uptime)
			msg := fmt.Sprintf("I have been running for %s", durafmt.Parse(dur))
			client.Say(config.ChannelName, msg)
		}

		if strings.HasPrefix(strings.ToLower(message.Message), "!miles") {
			log.Println(user, "ran !miles")
			// run if the user is a follower
			if mytwitch.UserIsFollower(user) {
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
			} else {
				client.Say(config.ChannelName, "You must be a follower to run that command :)")
			}
		}

		if strings.HasPrefix(strings.ToLower(message.Message), "!newmiles") {
			log.Println(user, "ran !newmiles")
			// run if the user is a follower
			if mytwitch.UserIsFollower(user) {
				miles := miles.ForUser(user)
				msg := ""
				switch {
				case miles == 1:
					msg = "@%s has only %.1f mile"
				case miles >= 250:
					msg = "Holy crap! @%s has %.1f miles!"
				default:
					msg = "@%s has %.1f miles"
				}
				// add the other part randomly
				if math.Rand(3) == 0 {
					msg = fmt.Sprintf("%s. Earn miles for every minute you watch the stream!", msg)
				}
				msg = fmt.Sprintf(msg, user, miles)
				client.Say(config.ChannelName, msg)
			} else {
				client.Say(config.ChannelName, "You must be a follower to run that command :)")
			}
		}

		if strings.HasPrefix(strings.ToLower(message.Message), "!optimized") {
			log.Println(user, "ran !optimized")
			// run if the user is a follower
			if mytwitch.UserIsFollower(user) {
				// get the currently-playing video
				currentVid := video.CurrentlyPlaying()
				if strings.Contains(currentVid, "_opt") {
					client.Say(config.ChannelName, "This video has been optimized")
				} else {
					client.Say(config.ChannelName, "This video is not yet optimized")
				}
			} else {
				client.Say(config.ChannelName, "You must be a follower to run that command :)")
			}
		}

		if strings.HasPrefix(strings.ToLower(message.Message), "!sunset") {
			log.Println(user, "ran !sunset")
			// run if the user is a follower
			if mytwitch.UserIsFollower(user) {
				// get the currently-playing video
				currentVid := video.CurrentlyPlaying()
				vid, err := video.New(currentVid)
				if err != nil {
					log.Println("unable to create Video: %v", err)
				}
				lat, lon, err := datastore.CoordsFor(vid)
				if err != nil {
					client.Say(config.ChannelName, "That didn't work, sorry!")
				} else {
					client.Say(config.ChannelName, helpers.SunsetStr(vid.Date(), lat, lon))
				}
			} else {
				client.Say(config.ChannelName, "You must be a follower to run that command :)")
			}
		}

		if strings.HasPrefix(strings.ToLower(message.Message), "!tripbot") {
			log.Println(user, "ran !tripbot")
			// run if the user is a follower
			if mytwitch.UserIsFollower(user) {
				// get the currently-playing video
				currentVid := video.CurrentlyPlaying()

				// only run if this video hasn't yet been processed
				if currentVid != lastVid {
					// extract the coordinates
					vid, err := video.New(currentVid)
					if err != nil {
						log.Println("unable to create Video: %v", err)
					}
					lat, lon, err := datastore.CoordsFor(vid)
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
			} else {
				client.Say(config.ChannelName, "You must be a follower to run that command :)")
			}

		}

		if strings.HasPrefix(strings.ToLower(message.Message), "!leaderboard") {
			log.Println(user, "ran !leaderboard")
			// run if the user is a follower
			if mytwitch.UserIsFollower(user) {
				userList := datastore.TopUsers(3)
				for i, leader := range userList {
					msg := fmt.Sprintf("#%d: %s (%dmi)", i+1, leader, datastore.MilesForUser(leader))
					client.Say(config.ChannelName, msg)
				}
			} else {
				client.Say(config.ChannelName, "You must be a follower to run that command :)")
			}
		}

		if strings.HasPrefix(strings.ToLower(message.Message), "!date") {
			log.Println(user, "ran !date")
			// run if the user is a follower
			if mytwitch.UserIsFollower(user) {
				// get the currently-playing video
				currentVid := video.CurrentlyPlaying()
				vid, err := video.New(currentVid)
				if err != nil {
					log.Println("unable to create video: %v", err)
				}
				lat, lon, err := datastore.CoordsFor(vid)
				if err != nil {
					client.Say(config.ChannelName, "That didn't work, sorry!")
				} else {
					realDate := helpers.ActualDate(vid.Date(), lat, lon)
					// "Mon, 02 Jan 2006 15:04:05 MST"
					fmtDate := realDate.Format(time.RFC1123)
					client.Say(config.ChannelName, fmt.Sprintf("This moment was %s", fmtDate))
				}
			} else {
				client.Say(config.ChannelName, "You must be a follower to run that command :)")
			}
		}

		if strings.HasPrefix(strings.ToLower(message.Message), "!state") {
			log.Println(user, "ran !state")
			// run if the user is a follower
			if mytwitch.UserIsFollower(user) {
				// get the currently-playing video
				currentVid := video.CurrentlyPlaying()
				vid, err := video.New(currentVid)
				if err != nil {
					log.Println("unable to create video: %v", err)
				}
				lat, lon, err := datastore.CoordsFor(vid)
				if err != nil {
					client.Say(config.ChannelName, "That didn't work, sorry!")
				} else {
					state, err := helpers.StateFromCoords(lat, lon)
					if err != nil || state == "" {
						client.Say(config.ChannelName, "That didn't work, sorry!")
					} else {
						msg := fmt.Sprintf("We're in %s", state)
						client.Say(config.ChannelName, msg)
					}
				}
			} else {
				client.Say(config.ChannelName, "You must be a follower to run that command :)")
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
