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
	"1174",
	"2020",
	"abbottcostello",
	"academyimpossible",
	"angeloflight",
	"anotherttvviewer",
	"apricotdrupefruit",
	"aten",
	"avocadobadado",
	"bibiethumps",
	"bingcortana",
	"brokemystreamdeck",
	"buglers",
	"carbob14xyz",
	"cartierlogic",
	"cashiering",
	"casinothanks",
	"cleaning_the_house",
	"clearyourbrowserhistory",
	"comettunes",
	"commanderroot",
	"community4smallstreamers",
	"communityshowcase",
	"cristianepre",
	"cyclemotion",
	"d1sc0rdforsmallstreamers",
	"d4rk_5how",
	"d4rk_5ky",
	"disc0rdforsma11streamers",
	"discord_for_streamers",
	"droopdoggg",
	"electricallongboard",
	"eubyt",
	"eulersobject",
	"extramoar",
	"feet",
	"feuerwehr",
	"flaskcopy",
	"freddyybot",
	"frw33d",
	"ftopayr",
	"ghrly",
	"gingerne",
	"gowithhim",
	"hades_osiris",
	"havethis2",
	"icantcontrolit",
	"icewizerds",
	"ildelara",
	"jdlb",
	"jeffecorga",
	"jobi_essen",
	"jointeffortt",
	"kishintern",
	"konkky",
	"lemonjuices12",
	"letsdothis_music",
	"logviewer",
	"lurxx",
	"mathgaming",
	"maybeilookoutofhiswindow",
	"minion619",
	"mrreflector",
	"mslenity",
	"music_and_arts",
	"n3td3v",
	"nerdydreams",
	"nightbot",
	"nuclearpigeons",
	"p0lizei_",
	"phantom_309",
	"playacted",
	"prankcher",
	"quavered",
	"rladmsdb88",
	"rogueg1rl",
	"rubberslayer",
	"sad_grl",
	"saddestkitty",
	"shadowy_stix",
	"sillygnome225",
	"slocool",
	"stickypigs",
	"stormmunity",
	"streamlabs",
	"stygian_styx",
	"talkingrobble",
	"taormina2600",
	"teresedirty",
	"teyyd",
	"tripbot4000",
	"twitchdetails",
	"twitchgrowthdiscord",
	"underworldnaiad",
	"unixchat",
	"utensilzinc",
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
