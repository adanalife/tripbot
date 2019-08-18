package tripbot

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/background"
	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/database"
	"github.com/dmerrick/danalol-stream/pkg/events"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/miles"
	"github.com/dmerrick/danalol-stream/pkg/video"
	"github.com/hako/durafmt"
)

func isFollower(user string) bool {
	// return mytwitch.UserIsFollower(user)
	//TODO: fixme
	return true
}

func help() string {
	text := config.HelpMessages[helpIndex]
	// bump the index
	helpIndex = (helpIndex + 1) % len(config.HelpMessages)
	return text
}

func helpCmd(user string) {
	log.Println(user, "ran !help")
	msg := fmt.Sprintf("%s (%d of %d)", help(), helpIndex+1, len(config.HelpMessages))
	client.Say(config.ChannelName, msg)
}

func uptimeCmd(user string) {
	log.Println(user, "ran !uptime")
	dur := time.Now().Sub(Uptime)
	msg := fmt.Sprintf("I have been running for %s", durafmt.Parse(dur))
	client.Say(config.ChannelName, msg)
}

func oldMilesCmd(user string) {
	log.Println(user, "ran !oldmiles")
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

func milesCmd(user string) {
	log.Println(user, "ran !miles")
	miles := miles.ForUser(user)
	msg := ""
	switch {
	case miles == 1:
		msg = "@%s has only %.1f mile."
	case miles >= 250:
		msg = "Holy crap! @%s has %.1f miles!"
	default:
		msg = "@%s has %.1f miles."
	}
	// add the other part randomly
	if rand.Intn(3) == 0 {
		msg = fmt.Sprintf("%s Earn miles for every minute you watch the stream!", msg)
	}
	msg = fmt.Sprintf(msg, user, miles)
	client.Say(config.ChannelName, msg)
}

func sunsetCmd(user string) {
	log.Println(user, "ran !sunset")
	// get the currently-playing video
	currentVid := video.CurrentlyPlaying
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
}

func locationCmd(user string) {
	log.Println(user, "ran !location (or similar)")
	// get the currently-playing video
	currentVid := video.CurrentlyPlaying

	// extract the coordinates
	vid, err := video.New(currentVid)
	if err != nil {
		log.Println("unable to create Video: %v", err)
	}
	lat, lon, err := datastore.CoordsFor(vid)
	if err != nil {
		client.Say(config.ChannelName, "I couldn't figure out the GPS coordinates... try again in ~3 minutes!")
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
}

func leaderboardCmd(user string) {
	log.Println(user, "ran !leaderboard")
	size := 10
	userList := miles.TopUsers(size)
	msg := fmt.Sprintf("Top %d miles: ", size)
	for i, leaderPair := range userList {
		msg += fmt.Sprintf("%d. %s (%s)", i+1, leaderPair[0], leaderPair[1])
		if i+1 != size {
			msg += ", "
		}
	}
	client.Say(config.ChannelName, msg)
}

func timeCmd(user string) {
	log.Println(user, "ran !time")
	// get the currently-playing video
	currentVid := video.CurrentlyPlaying
	vid, err := video.New(currentVid)
	if err != nil {
		log.Println("unable to create Video: %v", err)
	}
	lat, lon, err := datastore.CoordsFor(vid)
	if err != nil {
		client.Say(config.ChannelName, "That didn't work, sorry!")
	} else {
		realDate := helpers.ActualDate(vid.Date(), lat, lon)
		fmtTime := realDate.Format("3:04pm MST")
		client.Say(config.ChannelName, fmt.Sprintf("This moment was %s", fmtTime))
	}
}

func dateCmd(user string) {
	log.Println(user, "ran !date")
	// get the currently-playing video
	currentVid := video.CurrentlyPlaying
	vid, err := video.New(currentVid)
	if err != nil {
		log.Println("unable to create video: %v", err)
	}
	lat, lon, err := datastore.CoordsFor(vid)
	if err != nil {
		client.Say(config.ChannelName, "That didn't work, sorry!")
	} else {
		realDate := helpers.ActualDate(vid.Date(), lat, lon)
		fmtDate := realDate.Format("Monday January 2, 2006")
		client.Say(config.ChannelName, fmt.Sprintf("This moment was %s", fmtDate))
	}
}

func stateCmd(user string) {
	log.Println(user, "ran !state")
	// get the currently-playing video
	currentVid := video.CurrentlyPlaying
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
}

func reportCmd(user, message string) {
	log.Println(user, "ran !report")
	message = fmt.Sprintf("Report from Twitch Chat: %s", message)
	twilioClient.SendSMS(twilioFromNum, twilioToNum, message, "", "")
	client.Say(config.ChannelName, "Thank you, I will look into this ASAP!")
}

func shutdownCmd(user string) {
	log.Println(user, "ran !shutdown")
	if user != strings.ToLower(config.ChannelName) {
		client.Say(config.ChannelName, "I'm sorry, I won't do that")
		return
	}
	client.Say(config.ChannelName, "Shutting down...")
	log.Printf("currently playing: %s", video.CurrentlyPlaying)
	background.StopCron()
	events.LogoutAll(Uptime)
	database.DBCon.Close()
	os.Exit(0)
}

// Chatter will post a message to chat
func Chatter() {
	rand.Intn(len(config.HelpMessages))
	client.Say(config.ChannelName, help())
}
