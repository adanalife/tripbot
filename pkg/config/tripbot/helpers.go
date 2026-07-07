package config

import (
	"strings"
)

func (c TripbotConfig) IsProduction() bool {
	return c.Environment == "production"
}

func (c TripbotConfig) IsStaging() bool {
	return c.Environment == "staging"
}

func (c TripbotConfig) IsDevelopment() bool {
	return c.Environment == "development"
}

// UserIsAdmin returns true if a given user runs the channel
// it's used to restrict admin features
func UserIsAdmin(username string) bool {
	return strings.EqualFold(username, Conf.ChannelName)
}

// UserIsCompedSubscriber reports whether username is on the comped-subscriber
// allowlist — treated as a subscriber for subscriber-only commands without an
// actual sub.
func UserIsCompedSubscriber(username string) bool {
	for _, u := range Conf.CompedSubscribers {
		if strings.EqualFold(u, username) {
			return true
		}
	}
	return false
}

// HelpMessages are all of the different things !help can return
var HelpMessages = []string{
	"!commands: List more commands you can use",
	"!commands: List more commands you can use",
	"!commands: List more commands you can use",
	"!guess: Guess which state we are in",
	"!leaderboard: See who has the most miles",
	"!location: Get the current location",
	"!miles: See your current miles",
	"!report: Report a stream issue (frozen, no audio, etc)",
	"!state: Get the state we are currently in",
	"!sunset: Get time until sunset (on the day of filming)",
	"!timewarp: Magically warp to a new moment in time",
}

// YouTubeBotlessHelpMessages are the rotating Chatter / !help lines a bot-less
// YouTube instance posts (YOUTUBE_INBOUND_ENABLED=false). With no chat reader,
// the interactive commands can't respond, so these advertise the live Twitch
// channel and signal that YouTube interactivity is on the way. Deliberately
// free of any "!command" token: a command a YouTube viewer types into an unread
// chat looks like a broken bot. Swapped in by enabledHelpMessages when the App
// is bot-less.
var YouTubeBotlessHelpMessages = []string{
	"Want to chat? The interactive bot is live right now on Twitch → twitch.tv/ADanaLife_",
	"Interactive commands are coming to YouTube soon — subscribe so you don't miss it!",
	"Curious where we are or what day this was? Ask the bot live on Twitch: twitch.tv/ADanaLife_",
	"Driving across America, 24 hours a day. Watch and chat live on Twitch → twitch.tv/ADanaLife_",
	"This is a bot-powered slow-TV roadtrip. The full interactive experience is on Twitch: twitch.tv/ADanaLife_",
	"Guess the state, earn miles, climb the leaderboard — all live on Twitch: twitch.tv/ADanaLife_",
}
