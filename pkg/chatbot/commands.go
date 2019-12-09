package chatbot

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/audio"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"

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

func songCmd(user *users.User) {
	log.Println(user.Username, "ran !song")
	currentSong := audio.CurrentlyPlaying()
	// just print the link to somaFM if there's an issue
	if currentSong == "" {
		Say("https://somafm.com/groovesalad/songhistory.html")
		return
	}
	msg := fmt.Sprintf("We're listening to %s", currentSong)
	Say(msg)
}

func getHelpMessage(params []string) string {
	var msg string

	if len(params) == 0 {
		msg = "Greetings, I am Tripbot, your robot assistant. What would you like to do today? Try \"!help commands\", or \"!help location\" to get started."
		return msg
	}

	switch params[0] {
	case "commands":
		msg = "I offer many commands to inform and entertain you on our adventure. Some examples include: !location, !miles, !sunset, and !guess. To learn more about a command, use !help followed by the command name, like \"!help miles\" or \"!help state\"."
	case "location":
		msg = "The !location command gives you details about our current location. Related commands: !state, !guess."
	case "state":
		msg = "The !state command gives you the state we're currently in. Related commands: !location, !guess."
	case "guess":
		msg = "The !guess command lets you guess what state we're currently in. For example: \"!guess CA\" or \"!guess texas\". Try not to cheat! Related commands: !location, !state."
	case "miles":
		msg = "The !miles command lets you see your current miles. Miles are accumulated every minute you watch the stream. Related commands: !leaderboard, !kilometres."
	case "leaderboard":
		msg = "The !leaderboard command lets you see who has the most miles. Related commands: !miles, !kilometres."
	case "kilometres":
		msg = "The !kilometres command lets you see your current as kilometres. Miles/kilometres are accumulated every minute you watch the stream. Alias: !km. Related commands: !miles, !leaderboard."
	case "sunset":
		msg = "The !sunset command tells you the time until sunset on the date of filming. If it's after sunset, it will tell you how long it has been. Related commands: !date, !time."
	case "date":
		msg = "The !date command tells you the date the footage was filmed. Related command: !time."
	case "time":
		msg = "The !time command tells you the time (and timezone) in which the footage was filmed. Related command: !date."
	default:
		msg = "I'm not exactly sure what you mean. Try \"!help commands\" to learn more about what I can do for you."
	}

	return msg
}

func helpCmd(user *users.User, params []string, whisper bool) {
	log.Println(user.Username, "ran !help", params)

	msg := getHelpMessage(params)
	if whisper {
		client.Whisper(user.Username, msg)
	} else {
		client.Say(config.ChannelName, msg)
	}
}

func uptimeCmd(user *users.User) {
	log.Println(user.Username, "ran !uptime")
	dur := time.Now().Sub(Uptime)
	msg := fmt.Sprintf("I have been running for %s", durafmt.Parse(dur))
	Say(msg)
}

func milesCmd(user *users.User) {
	log.Println(user.Username, "ran !miles")
	miles := user.CurrentMiles()
	msg := "@%s has %.2f miles."
	msg = fmt.Sprintf(msg, user.Username, miles)
	if miles < 0.1 {
		msg += " You'll earn more miles every minute you watch the stream."
	}
	Say(msg)
}

func kilometresCmd(user *users.User) {
	log.Println(user.Username, "ran !kilometres")
	km := user.CurrentMiles() * 1.609344
	msg := "@%s has %.2f kilometres."
	msg = fmt.Sprintf(msg, user.Username, km)
	Say(msg)
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
	Say(msg)
}

func sunsetCmd(user *users.User) {
	log.Println(user.Username, "ran !sunset")
	// get the currently-playing video
	vid := video.CurrentlyPlaying
	if vid.Flagged {
		Say("I couldn't figure out current GPS coords, using next closest...")
		vid = vid.Next()
	}
	lat, lng, _ := vid.Location()
	Say(helpers.SunsetStr(vid.DateFilmed, lat, lng))
}

func locationCmd(user *users.User) {
	log.Println(user.Username, "ran !location (or similar)")
	// get the currently-playing video
	vid := video.CurrentlyPlaying
	if vid.Flagged {
		Say("I couldn't figure out current GPS coords, using next closest...")
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
	Say(msg)
}

func leaderboardCmd(user *users.User) {
	log.Println(user.Username, "ran !leaderboard")
	// display leaderboard on screen
	background.ShowLeaderboard()
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
	Say(msg)
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
	Say(msg)
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
		Say("I couldn't figure out current GPS coords, sorry!")
	} else {
		realDate := helpers.ActualDate(vid.DateFilmed, lat, lng)
		fmtTime := realDate.Format("3:04pm MST")
		Say(fmt.Sprintf("This moment was %s", fmtTime))
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
		Say("I couldn't figure out current GPS coords, sorry!")
	} else {
		realDate := helpers.ActualDate(vid.DateFilmed, lat, lng)
		fmtDate := realDate.Format("Monday January 2, 2006")
		Say(fmt.Sprintf("This moment was %s", fmtDate))
	}
}

//TODO: refactor to use golang '...' syntax
func guessCmd(user *users.User, params []string) {
	log.Println(user.Username, "ran !guess")
	var msg string

	if len(params) == 0 {
		msg = "Try and guess what state we're in! For example: !guess CA"
		Say(msg)
		return
	}

	// get the arg from the command
	guess := strings.Join(params, " ")

	// convert to short form if they used the full name
	// e.g. "Massachusetts" instead of "MA"
	if len(guess) == 2 {
		guess = stateAbbrevs[strings.ToUpper(guess)]
	}

	// get the currently-playing video
	vid := video.CurrentlyPlaying
	if vid.Flagged {
		Say("I couldn't figure out current GPS coords, using next closest...")
		vid = vid.Next()
	}

	if strings.ToLower(guess) == strings.ToLower(vid.State) {
		msg = fmt.Sprintf("@%s got it! We're in %s", user.Username, vid.State)
	} else {
		msg = "Try again! EarthDay"
	}
	Say(msg)
}

func stateCmd(user *users.User) {
	log.Println(user.Username, "ran !state")
	// get the currently-playing video
	vid := video.CurrentlyPlaying
	if vid.Flagged {
		Say("I couldn't figure out current GPS coords, using next closest...")
		vid = vid.Next()
	}
	msg := fmt.Sprintf("We're in %s", vid.State)
	Say(msg)
}

//TODO: maybe there could be a !cancel command or something
//TODO: use fancy golang ... syntax?
func reportCmd(user *users.User, params []string) {
	log.Println(user.Username, "ran !report")
	message := strings.Join(params, " ")
	message = fmt.Sprintf("Report from Twitch Chat: %s", message)
	helpers.SendSMS(message)
	Say("Thank you, I will look into this ASAP!")
}

func bonusMilesCmd(user *users.User) {
	log.Println(user.Username, "ran !bonusmiles")
	bonus := user.BonusMiles()
	msg := fmt.Sprintf("%s has earned %.4f bonus miles this session", user.Username, bonus)
	Say(msg)
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
	Say(msg)
}

func shutdownCmd(user *users.User) {
	log.Println(user.Username, "ran !shutdown")
	if user.Username != strings.ToLower(config.ChannelName) {
		Say("Nice try bucko")
		return
	}
	Say("Shutting down...")
	log.Printf("currently playing: %s", video.CurrentlyPlaying)
	background.StopCron()
	users.Shutdown()
	err := database.DBCon.Close()
	if err != nil {
		log.Println(err)
	}
	audio.Shutdown()
	sentry.Flush(time.Second * 5)
	os.Exit(0)
}
