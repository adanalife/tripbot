package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/dmerrick/danalol-stream/pkg/screenshot"
	"github.com/dmerrick/danalol-stream/pkg/store"
	twitch "github.com/gempir/go-twitch-irc"
)

const (
	clientUsername = "TripBot4000"
	channelToJoin  = "adanalife_"
)

// these are other bots who shouldn't get points
// https://twitchinsights.net/bots
var ignoredUsers = []string{
	"nightbot",
	"anotherttvviewer",
	"apricotdrupefruit",
	"commanderroot",
	"communityshowcase",
	"electricallongboard",
	"logviewer",
	"lurxx",
	"p0lizei_",
	"slocool",
	"unixchat",
	"v_and_k",
	"virgoproz",
	"zanekyber",
	"feuerwehr",
	"jobi_essen",
	"freddyybot",
	"taormina2600",
	"avocadobadado",
	"eubyt",
}

// the most-recently processed video
var lastVid string

func main() {
	// first we must check for required ENV vars
	clientAuthenticationToken, ok := os.LookupEnv("TWITCH_AUTH_TOKEN")
	if !ok {
		panic("You must set TWITCH_AUTH_TOKEN")
	}

	// initialize the database
	datastore := store.NewStore("tripbot.db")
	if err := datastore.Open(); err != nil {
		panic(err)
	}

	// catch CTRL-Cs and run datastore.Close()
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		datastore.Close()
		os.Exit(1)
	}()

	// show the DB contents at the start
	datastore.PrintStats()

	// time to set up the Twitch client
	client := twitch.NewClient(clientUsername, clientAuthenticationToken)

	client.OnUserJoinMessage(func(joinMessage twitch.UserJoinMessage) {
		if !userIsIgnored(joinMessage.User) {
			datastore.RecordUserJoin(joinMessage.User)
		}
	})

	client.OnUserPartMessage(func(partMessage twitch.UserPartMessage) {
		if !userIsIgnored(partMessage.User) {
			datastore.RecordUserPart(partMessage.User)
		}
	})

	client.OnWhisperMessage(func(message twitch.WhisperMessage) {
		log.Println("whisper:", message.User.Name, message.Message)
	})

	// all chat messages
	client.OnPrivateMessage(func(message twitch.PrivateMessage) {
		if strings.HasPrefix(strings.ToLower(message.Message), "!tripbot") {

			// get the currently-playing video
			currentVid := screenshot.GetCurrentVideo()

			// only run once every 3 minutes
			if currentVid != lastVid {
				screenshotPath := screenshot.ScreenshotPath(currentVid)
				// extract the coordinates, generate a google maps url
				url, err := screenshot.ProcessImage(screenshotPath)
				if err != nil {
					client.Say(channelToJoin, "Sorry, it didn't work this time :(. Try again in a few minutes!")
				} else {
					client.Say(channelToJoin, fmt.Sprintf("If this doesn't work, try again in a few minutes: %s", url))
				}
				// update the last vid
				lastVid = currentVid
			}

		}
	})

	// join the channel
	client.Join(channelToJoin)
	log.Println("Joined channel", channelToJoin)

	// actually connect to Twitch
	err := client.Connect()
	if err != nil {
		panic(err)
	}
}

// returns true if a given user should be ignored
func userIsIgnored(user string) bool {
	for _, ignored := range ignoredUsers {
		if user == ignored {
			return true
		}
	}
	return false
}
