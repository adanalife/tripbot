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

// UserIsIgnored returns true if a given user should be ignored
func UserIsIgnored(user string) bool {
	for _, ignored := range IgnoredUsers {
		if user == ignored {
			return true
		}
	}
	return false
}

//TODO: this should load from a config file
// IgnoredUsers are users who shouldn't be in the running for miles
// https://twitchinsights.net/bots
var IgnoredUsers = []string{
	"0_applebadapple_0",
	"abbottcostello",
	"angeloflight",
	"anotherttvviewer",
	"apricotdrupefruit",
	"aten",
	"avocadobadado",
	"bibiethumps",
	"bingcortana",
	"cartierlogic",
	"casinothanks",
	"clearyourbrowserhistory",
	"commanderroot",
	"communityshowcase",
	"cristianepre",
	"cyclemotion",
	"droopdoggg",
	"electricallongboard",
	"eubyt",
	"eulersobject",
	"extramoar",
	"feet",
	"feuerwehr",
	"flaskcopy",
	"freddyybot",
	"ftopayr",
	"ghrly",
	"gingerne",
	"gowithhim",
	"havethis2",
	"icewizerds",
	"jeffecorga",
	"jobi_essen",
	"jointeffortt",
	"kishintern",
	"konkky",
	"letsdothis_music",
	"logviewer",
	"lurxx",
	"mathgaming",
	"maybeilookoutofhiswindow",
	"minion619",
	"mrreflector",
	"mslenity",
	"n3td3v",
	"nightbot",
	"nuclearpigeons",
	"p0lizei_",
	"prankcher",
	"rladmsdb88",
	"sad_grl",
	"saddestkitty",
	"sillygnome225",
	"slocool",
	"streamlabs",
	"talkingrobble",
	"taormina2600",
	"teresedirty",
	"teyyd",
	"tripbot4000",
	"unixchat",
	"v_and_k",
	"violets_tv",
	"virgoproz",
	"winsock",
	"zanekyber",
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
	"!survey: Fill out a survey and help the stream",
	"!timewarp: Magically warp to a new moment in time",
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
