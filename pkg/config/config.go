package config

import (
	"log"
	"os"
	"path"
	"strconv"

	"github.com/joho/godotenv"
)

const (
	// ChannelID is the internal twitch ID of the channel
	ChannelID = "225469317"

	screencapDir = "screencaps"
	videoDir     = "_all"

	// MapsOutputDir is where the maps script saves the frames
	MapsOutputDir = "/Volumes/usbshare1/maps"
	// CroppedPath is where we store the cropped versions of screencaps (to OCR them)
	CroppedPath = "/Volumes/Luna/cropped-corners"

	DBPath            = "db/tripbot.db"
	UserJoinsBucket   = "user_joins"
	UserWatchedBucket = "user_watched"
	CoordsBucket      = "coords"
)

var ChannelName string
var ReadOnly bool
var Verbose bool

func init() {
	// load ENV vars from .env file
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	ChannelName = os.Getenv("CHANNEL_NAME")
	ReadOnly, _ = strconv.ParseBool(os.Getenv("READ_ONLY"))
	Verbose, _ = strconv.ParseBool(os.Getenv("VERBOSE"))
}

func VideoDir() string {
	return path.Join(os.Getenv("DASHCAM_DIR"), videoDir)
}

func ScreencapDir() string {
	return path.Join(os.Getenv("DASHCAM_DIR"), screencapDir)
}

//TODO: this should load from a config file
// IgnoredUsers are users who shouldn't be in the running for miles
// https://twitchinsights.net/bots
var IgnoredUsers = []string{
	"adanalife_",
	"tripbot4000",
	"nightbot",
	"streamlabs",
	"0_applebadapple_0",
	"angeloflight",
	"anotherttvviewer",
	"apricotdrupefruit",
	"avocadobadado",
	"commanderroot",
	"communityshowcase",
	"cyclemotion",
	"electricallongboard",
	"eubyt",
	"feuerwehr",
	"freddyybot",
	"jobi_essen",
	"konkky",
	"logviewer",
	"lurxx",
	"mrreflector",
	"n3td3v",
	"p0lizei_",
	"slocool",
	"taormina2600",
	"teyyd",
	"unixchat",
	"v_and_k",
	"virgoproz",
	"zanekyber",
}

// HelpMessages are all of the different things !help can return
var HelpMessages = []string{
	"!location: Get the current location (beta)",
	"!map: Show a map of the whole trip",
	"!info: Get more details on the footage",
	"!song: Get the current music",
	"!miles: See your current miles",
	"!leaderboard: See who has the most miles",
	"!state: Get the state we are currently in (beta)",
	"!sunset: Get time until sunset (on the day of filming)",
	"!report: Report a stream issue (frozen, no audio, etc)",
	"!temperature: Will be unlocked when the donation goal is reached",
	"!guess: Guess which state we are in",
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
