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

// Chatter will post a message to chat
func Chatter() {
	rand.Intn(len(config.HelpMessages))
	client.Say(config.ChannelName, help())
}

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
	vid := video.CurrentlyPlaying
	lat, lon, err := vid.Location()
	if err != nil {
		client.Say(config.ChannelName, "I couldn't figure out current GPS coords, sorry!")
	} else {
		client.Say(config.ChannelName, helpers.SunsetStr(vid.DateFilmed, lat, lon))
	}
}

func locationCmd(user string) {
	log.Println(user, "ran !location (or similar)")
	// get the currently-playing video
	vid := video.CurrentlyPlaying
	// extract the coordinates
	lat, lon, err := vid.Location()
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
	vid := video.CurrentlyPlaying
	lat, lon, err := vid.Location()
	if err != nil {
		client.Say(config.ChannelName, "I couldn't figure out current GPS coords, sorry!")
	} else {
		realDate := helpers.ActualDate(vid.DateFilmed, lat, lon)
		fmtTime := realDate.Format("3:04pm MST")
		client.Say(config.ChannelName, fmt.Sprintf("This moment was %s", fmtTime))
	}
}

func dateCmd(user string) {
	log.Println(user, "ran !date")
	// get the currently-playing video
	vid := video.CurrentlyPlaying
	lat, lon, err := vid.Location()
	if err != nil {
		client.Say(config.ChannelName, "I couldn't figure out current GPS coords, sorry!")
	} else {
		realDate := helpers.ActualDate(vid.DateFilmed, lat, lon)
		fmtDate := realDate.Format("Monday January 2, 2006")
		client.Say(config.ChannelName, fmt.Sprintf("This moment was %s", fmtDate))
	}
}

func guessCmd(user, message string) {
	log.Println(user, "ran !guess")
	var msg string

	if len(message) <= 7 {
		msg = "Try and guess what state we're in! For example: !guess CA"
		client.Say(config.ChannelName, msg)
		return
	}

	// get the arg from the command
	// there might be a better way to do this
	parts := strings.Split(message, " ")
	guess := strings.Join(parts[1:], " ")

	// convert to short form if necessary
	if len(guess) == 2 {
		guess = stateAbbrevs[strings.ToUpper(guess)]
	}

	// get the currently-playing video
	vid := video.CurrentlyPlaying
	if vid.Flagged {
		client.Say(config.ChannelName, "I couldn't figure out current GPS coords, sorry!")
	} else {
		if strings.ToLower(guess) == strings.ToLower(vid.State) {
			msg = fmt.Sprintf("@%s got it! We're in %s", user, vid.State)
		} else {
			msg = "Try again! EarthDay"
		}
		client.Say(config.ChannelName, msg)
	}
}

func stateCmd(user string) {
	log.Println(user, "ran !state")
	// get the currently-playing video
	vid := video.CurrentlyPlaying
	if vid.Flagged {
		client.Say(config.ChannelName, "I couldn't figure out current GPS coords, sorry!")
	} else {
		msg := fmt.Sprintf("We're in %s", vid.State)
		client.Say(config.ChannelName, msg)
	}
}

//TODO: maybe there could be a !cancel command or something
func reportCmd(user, message string) {
	log.Println(user, "ran !report")
	message = fmt.Sprintf("Report from Twitch Chat: %s", message)
	twilioClient.SendSMS(twilioFromNum, twilioToNum, message, "", "")
	client.Say(config.ChannelName, "Thank you, I will look into this ASAP!")
}

func secretInfoCmd(user string) {
	log.Println(user, "ran !secretinfo")
	if user != strings.ToLower(config.ChannelName) {
		return
	}
	vid := video.CurrentlyPlaying
	msg := fmt.Sprintf("currently playing: %s, playtime: %s", vid, video.CurrentProgress())
	lat, lon, err := vid.Location()
	if err != nil {
		msg = fmt.Sprintf("%s, err: %s", msg, err)
	} else {
		msg = fmt.Sprintf("%s, lat: %f, lng: %f", msg, lat, lon)
	}
	log.Println(msg)
	client.Say(config.ChannelName, msg)
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
