package main

import (
	"log"
	"os"
	"time"

	twitch "github.com/gempir/go-twitch-irc"
)

const (
	clientUsername = "TripBot4000"
	channelToJoin  = "adanalife_"
)

// these are other bots who shouldn't get points
var ignoredUsers = []string{
	"anotherttvviewer",
	"commanderroot",
	"electricallongboard",
	"logviewer",
}

// datastore for user joins
var userJoins map[string]time.Time = make(map[string]time.Time)
var userWatched map[string]time.Duration = make(map[string]time.Duration)

// returns true if a given user should be ignored
func userIsIgnored(user string) bool {
	for _, ignored := range ignoredUsers {
		if user == ignored {
			return true
		}
	}
	return false
}

func recordUserJoin(user string) {
	userJoins[user] = time.Now()
	log.Println(user, "joined the channel")
}

func recordUserPart(user string) {
	if joinTime, ok := userJoins[user]; ok {
		duration := time.Since(joinTime)
		userWatched[user] += duration
	} else {
		log.Println("Hmm, part message with no join for user", user)
	}
	log.Println(user, "left the channel, total watched:", fmtDuration(userWatched[user]))
}

// helper func to make Durations prettier
func fmtDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	return fmt.Sprintf("%02d:%02d", h, m)
}

func main() {
	clientAuthenticationToken, ok := os.LookupEnv("TWITCH_AUTH_TOKEN")
	if !ok {
		panic("You must set TWITCH_AUTH_TOKEN")
	}

	client := twitch.NewClient(clientUsername, clientAuthenticationToken)

	client.OnUserJoinMessage(func(joinMessage twitch.UserJoinMessage) {
		if !userIsIgnored(joinMessage.User) {
			recordUserJoin(joinMessage.User)
			// log.Println(joinMessage.Raw)
		}
	})

	client.OnUserPartMessage(func(partMessage twitch.UserPartMessage) {
		if !userIsIgnored(partMessage.User) {
			recordUserPart(partMessage.User)
			// log.Println(partMessage.Raw)
		}
	})

	client.Join(channelToJoin)
	log.Println("Joined channel", channelToJoin)

	err := client.Connect()
	if err != nil {
		panic(err)
	}
}
