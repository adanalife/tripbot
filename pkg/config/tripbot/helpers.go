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

func (c TripbotConfig) IsTesting() bool {
	return c.Environment == "testing"
}

// UserIsAdmin returns true if a given user runs the channel
// it's used to restrict admin features
func UserIsAdmin(username string) bool {
	return strings.ToLower(username) == strings.ToLower(Conf.ChannelName)
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
	"Interactive commands are coming to YouTube soon — follow so you don't miss it!",
	"Curious where we are or what day this was? Ask the bot live on Twitch: twitch.tv/ADanaLife_",
	"Driving across America, 24 hours a day. Watch and chat live on Twitch → twitch.tv/ADanaLife_",
	"This is a bot-powered slow-TV roadtrip. The full interactive experience is on Twitch: twitch.tv/ADanaLife_",
}

var GoogleMapsStyle = []string{
	"element:geometry|color:0x242f3e",
	"element:labels.text.fill|color:0x746855",
	"element:labels.text.stroke|color:0x242f3e",
	"feature:administrative.locality|element:labels.text.fill|color:0xd59563",
	"feature:poi|element:labels.text.fill|color:0xd59563",
	"feature:poi.park|element:geometry|color:0x263c3f",
	"feature:poi.park|element:labels.text.fill|color:0x6b9a76",
	"feature:road|element:geometry|color:0x38414e",
	"feature:road|element:geometry.stroke|color:0x212a37",
	"feature:road|element:labels.text.fill|color:0x9ca5b3",
	"feature:road.highway|element:geometry|color:0x746855",
	"feature:road.highway|element:geometry.stroke|color:0x1f2835",
	"feature:road.highway|element:labels.text.fill|color:0xf3d19c",
	"feature:transit|element:geometry|color:0x2f3948",
	"feature:transit.station|element:labels.text.fill|color:0xd59563",
	"feature:water|element:geometry|color:0x17263c",
	"feature:water|element:labels.text.fill|color:0x515c6d",
	"feature:water|element:labels.text.stroke|color:0x17263c&size=480x360",
}

// these are different timestamps we have screenshots prepared for
// the "000" corresponds to 0m0s, "130" corresponds to 1m30s
var TimestampsToTry = []string{
	"000",
	"015",
	"030",
	"045",
	"100",
	"115",
	"130",
	"145",
	"200",
	"215",
	"230",
	"245",
}
