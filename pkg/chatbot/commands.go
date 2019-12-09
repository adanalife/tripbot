package chatbot

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/nicklaw5/helix"

	"github.com/dmerrick/danalol-stream/pkg/background"
	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/database"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/miles"
	"github.com/dmerrick/danalol-stream/pkg/users"
	"github.com/dmerrick/danalol-stream/pkg/video"
	"github.com/getsentry/sentry-go"
	"github.com/hako/durafmt"
)

func helpCmd(user *users.User) {
	log.Println(user.Username, "ran !help")
	msg := fmt.Sprintf("%s (%d of %d)", help(), helpIndex+1, len(config.HelpMessages))
	client.Say(config.ChannelName, msg)
}

func uptimeCmd(user *users.User) {
	log.Println(user.Username, "ran !uptime")
	dur := time.Now().Sub(Uptime)
	msg := fmt.Sprintf("I have been running for %s", durafmt.Parse(dur))
	client.Say(config.ChannelName, msg)
}

func milesCmd(user *users.User) {
	log.Println(user.Username, "ran !miles")
	miles := user.CurrentMiles()
	msg := "@%s has %.2f miles."
	msg = fmt.Sprintf(msg, user.Username, miles)
	if miles < 0.1 {
		msg += " You'll earn more miles every minute you watch the stream."
	}
	client.Say(config.ChannelName, msg)
}

func kilometresCmd(user *users.User) {
	log.Println(user.Username, "ran !kilometres")
	km := user.CurrentMiles() * 1.609344
	msg := "@%s has %.2f kilometres."
	msg = fmt.Sprintf(msg, user.Username, km)
	client.Say(config.ChannelName, msg)
}

func oldMilesCmd(user *users.User) {
	log.Println(user.Username, "ran !oldmiles")
	miles := miles.ForUser(user.Username)
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
	msg = fmt.Sprintf(msg, user.Username, miles)
	client.Say(config.ChannelName, msg)
}

func sunsetCmd(user *users.User) {
	log.Println(user.Username, "ran !sunset")
	// get the currently-playing video
	vid := video.CurrentlyPlaying
	if vid.Flagged {
		client.Say(config.ChannelName, "I couldn't figure out current GPS coords, using next closest...")
		vid = vid.Next()
	}
	lat, lng, _ := vid.Location()
	client.Say(config.ChannelName, helpers.SunsetStr(vid.DateFilmed, lat, lng))
}

func locationCmd(user *users.User) {
	log.Println(user.Username, "ran !location (or similar)")
	// get the currently-playing video
	vid := video.CurrentlyPlaying
	if vid.Flagged {
		client.Say(config.ChannelName, "I couldn't figure out current GPS coords, using next closest...")
		//TODO: write something like vid.FindClosest() that
		// chooses whether or not to use Next() vs Prev()
		vid = vid.Next()
	}
	// extract the coordinates
	lat, lng, err := vid.Location()
	// geocode the location
	address, _ := helpers.CityFromCoords(lat, lng)
	if err != nil {
		terrors.Log(err, "geocoding error")
	}
	// generate a google maps url
	url := helpers.GoogleMapsURL(lat, lng)
	msg := fmt.Sprintf("%s %s", address, url)
	client.Say(config.ChannelName, msg)
}

func leaderboardCmd(user *users.User) {
	log.Println(user.Username, "ran !leaderboard")
	size := 10
	if size > len(users.Leaderboard) {
		size = len(users.Leaderboard)
	}
	leaderboard := users.Leaderboard[:size]
	msg := fmt.Sprintf("Top %d miles: ", size)
	for i, leaderPair := range leaderboard {
		msg += fmt.Sprintf("%d. %s (%s)", i+1, leaderPair[0], leaderPair[1])
		if i+1 != len(leaderboard) {
			msg += ", "
		}
	}
	client.Say(config.ChannelName, msg)
}

func oldLeaderboardCmd(user *users.User) {
	log.Println(user.Username, "ran !oldleaderboard")
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

func timeCmd(user *users.User) {
	log.Println(user.Username, "ran !time")
	var err error
	var lat, lng float64
	// get the currently-playing video
	vid := video.CurrentlyPlaying
	if vid.Flagged {
		// use the location from the next vid
		lat, lng, err = vid.Next().Location()
	} else {
		lat, lng, err = vid.Location()
	}
	if err != nil {
		// why would we get in here?
		client.Say(config.ChannelName, "I couldn't figure out current GPS coords, sorry!")
	} else {
		realDate := helpers.ActualDate(vid.DateFilmed, lat, lng)
		fmtTime := realDate.Format("3:04pm MST")
		client.Say(config.ChannelName, fmt.Sprintf("This moment was %s", fmtTime))
	}
}

func dateCmd(user *users.User) {
	log.Println(user.Username, "ran !date")
	var err error
	var lat, lng float64
	// get the currently-playing video
	vid := video.CurrentlyPlaying
	if vid.Flagged {
		// use the location from the next vid
		lat, lng, err = vid.Next().Location()
	} else {
		lat, lng, err = vid.Location()
	}
	if err != nil {
		// why would we get in here?
		client.Say(config.ChannelName, "I couldn't figure out current GPS coords, sorry!")
	} else {
		realDate := helpers.ActualDate(vid.DateFilmed, lat, lng)
		fmtDate := realDate.Format("Monday January 2, 2006")
		client.Say(config.ChannelName, fmt.Sprintf("This moment was %s", fmtDate))
	}
}

//TODO: refactor to use golang '...' syntax
func guessCmd(user *users.User, params []string) {
	log.Println(user.Username, "ran !guess")
	var msg string

	if len(params) == 0 {
		msg = "Try and guess what state we're in! For example: !guess CA"
		client.Say(config.ChannelName, msg)
		return
	}

	// get the arg from the command
	guess := strings.Join(params[1:], " ")

	// convert to short form if they used the full name
	// e.g. "Massachusetts" instead of "MA"
	if len(guess) == 2 {
		guess = stateAbbrevs[strings.ToUpper(guess)]
	}

	// get the currently-playing video
	vid := video.CurrentlyPlaying
	if vid.Flagged {
		client.Say(config.ChannelName, "I couldn't figure out current GPS coords, using next closest...")
		vid = vid.Next()
	}

	if guess == strings.ToLower(vid.State) {
		msg = fmt.Sprintf("@%s got it! We're in %s", user.Username, vid.State)
	} else {
		msg = "Try again! EarthDay"
	}
	client.Say(config.ChannelName, msg)
}

func stateCmd(user *users.User) {
	log.Println(user.Username, "ran !state")
	// get the currently-playing video
	vid := video.CurrentlyPlaying
	if vid.Flagged {
		client.Say(config.ChannelName, "I couldn't figure out current GPS coords, using next closest...")
		vid = vid.Next()
	}
	msg := fmt.Sprintf("We're in %s", vid.State)
	client.Say(config.ChannelName, msg)
}

//TODO: maybe there could be a !cancel command or something
func reportCmd(user *users.User, message string) {
	log.Println(user.Username, "ran !report")
	message = fmt.Sprintf("Report from Twitch Chat: %s", message)
	helpers.SendSMS(message)
	client.Say(config.ChannelName, "Thank you, I will look into this ASAP!")
}

func bonusMilesCmd(user *users.User) {
	log.Println(user.Username, "ran !bonusmiles")
	bonus := user.BonusMiles()
	msg := fmt.Sprintf("%s has earned %.4f bonus miles this session", user.Username, bonus)
	client.Say(config.ChannelName, msg)
}

func secretInfoCmd(user *users.User) {
	log.Println(user.Username, "ran !secretinfo")
	if user.Username != strings.ToLower(config.ChannelName) {
		return
	}
	vid := video.CurrentlyPlaying
	msg := fmt.Sprintf("currently playing: %s, playtime: %s", vid, video.CurrentProgress())
	lat, lng, err := vid.Location()
	if err != nil {
		msg = fmt.Sprintf("%s, err: %s", msg, err)
	} else {
		msg = fmt.Sprintf("%s, lat: %f, lng: %f", msg, lat, lng)
	}
	log.Println(msg)
	client.Say(config.ChannelName, msg)
}

func shutdownCmd(user *users.User) {
	log.Println(user.Username, "ran !shutdown")
	if user.Username != strings.ToLower(config.ChannelName) {
		client.Say(config.ChannelName, "Nice try bucko")
		return
	}
	client.Say(config.ChannelName, "Shutting down...")
	log.Printf("currently playing: %s", video.CurrentlyPlaying)
	background.StopCron()
	users.Shutdown()
	database.DBCon.Close()
	sentry.Flush(time.Second * 5)
	os.Exit(0)
}
