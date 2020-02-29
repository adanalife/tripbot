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
	vlcClient "github.com/dmerrick/danalol-stream/pkg/vlc-client"
	"github.com/getsentry/sentry-go"
	"github.com/hako/durafmt"
)

var lastTimewarpTime time.Time

func helpCmd(user *users.User) {
	log.Println(user.Username, "ran !help")
	msg := fmt.Sprintf("%s (%d of %d)", help(), helpIndex+1, len(config.HelpMessages))
	Say(msg)
}

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

func timewarpCmd(user *users.User) {
	log.Println(user.Username, "ran !timewarp")

	// exit early if we're on OS X
	if helpers.RunningOnDarwin() {
		Say("Sorry, timewarp isn't available right now")
		return
	}

	// rate-limit the number of times this can run
	if helpers.UserIsAdmin(user.Username) {
		if time.Now().Sub(lastTimewarpTime) < 20*time.Second {
			Say("Not yet; enjoy the moment!")
			return
		}
	}

	// only say this if the caller is not me
	if helpers.UserIsAdmin(user.Username) {
		Say("Here we go...!")
	}

	// show timewarp onscreen
	background.ShowTimewarp()
	// shuffle to a new //video
	vlcClient.PlayRandom()
	// update the currently-playing video
	video.GetCurrentlyPlaying()
	// update our record of last time it ran
	lastTimewarpTime = time.Now()
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
	if helpers.UserIsAdmin(user.Username) {
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
	if helpers.UserIsAdmin(user.Username) {
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

func restartMusicCmd(user *users.User) {
	log.Println(user.Username, "ran !restartmusic")
	if helpers.UserIsAdmin(user.Username) {
		Say("You can't do that, but please !report any stream issues")
		return
	}

	Say("Restarting music player...")
	if helpers.RunningOnDarwin() {
		audio.RestartItunes()
	} else {
		audio.StartGrooveSalad()
	}
}

// middleCmd sets the text at the bottom-middle of the stream
func middleCmd(user *users.User, params []string) {
	log.Println(user.Username, "ran !middle")
	// don't let strangers run this
	if helpers.UserIsAdmin(user.Username) {
		return
	}

	// don't do anything if empty
	if len(params) == 0 {
		Say("What do you want to say?")
		return
	}

	// use the params as the text
	text := strings.Join(params, " ")

	// just to help debug
	log.Printf("setting middle text to: %s", text)

	background.MiddleText.Show(text)
}
