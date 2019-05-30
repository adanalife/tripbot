package tripbot

import (
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/miles"
	mytwitch "github.com/dmerrick/danalol-stream/pkg/twitch"
	"github.com/dmerrick/danalol-stream/pkg/video"
	"github.com/hako/durafmt"
)

func isFollower(user string) bool {
	return mytwitch.UserIsFollower(user)
}

func helpCmd(user string) {
	log.Println(user, "ran !help")
	msg := fmt.Sprintf("%s (%d of %d)", config.HelpMessages[helpIndex], helpIndex+1, len(config.HelpMessages))
	client.Say(config.ChannelName, msg)
	// bump the index
	helpIndex = (helpIndex + 1) % len(config.HelpMessages)
}

func uptimeCmd(user string) {
	log.Println(user, "ran !uptime")
	dur := time.Now().Sub(Uptime)
	msg := fmt.Sprintf("I have been running for %s", durafmt.Parse(dur))
	client.Say(config.ChannelName, msg)
}

func optimizedCmd(user string) {
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

func oldMilesCmd(user string) {
	log.Println(user, "ran !oldmiles")
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

func milesCmd(user string) {
	log.Println(user, "ran !miles")
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
		if rand.Intn(3) == 0 {
			msg = fmt.Sprintf("%s. Earn miles for every minute you watch the stream!", msg)
		}
		msg = fmt.Sprintf(msg, user, miles)
		client.Say(config.ChannelName, msg)
	} else {
		client.Say(config.ChannelName, "You must be a follower to run that command :)")
	}
}

func sunsetCmd(user string) {
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

func tripbotCmd(user string) {
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

func leaderboardCmd(user string) {
	log.Println(user, "ran !leaderboard")
	// run if the user is a follower
	if mytwitch.UserIsFollower(user) {
		userList := miles.TopUsers(3)
		for i, leaderPair := range userList {
			msg := fmt.Sprintf("#%d: %s (%smi)", i+1, leaderPair[0], leaderPair[1])
			client.Say(config.ChannelName, msg)
		}
	} else {
		client.Say(config.ChannelName, "You must be a follower to run that command :)")
	}
}

func timeCmd(user string) {
	log.Println(user, "ran !time")
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
			realDate := helpers.ActualDate(vid.Date(), lat, lon)
			// "15:04 MST"
			fmtTime := realDate.Format("1:04AM MST")
			client.Say(config.ChannelName, fmt.Sprintf("This moment was %s", fmtTime))
		}
	} else {
		client.Say(config.ChannelName, "You must be a follower to run that command :)")
	}
}

func dateCmd(user string) {
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

func stateCmd(user string) {
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
