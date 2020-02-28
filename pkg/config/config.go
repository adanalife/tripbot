package config

import (
	"log"
	"os"
	"path"
	"strconv"

	"github.com/joho/godotenv"
)

const (
	// these are the default subdirectories
	screencapDir = "screencaps"
	videoDir     = "_all"

	DBPath            = "db/tripbot.db"
	UserJoinsBucket   = "user_joins"
	UserWatchedBucket = "user_watched"
	CoordsBucket      = "coords"
)

//TODO: add consistency between use of Dir vs Path in these names
var (
	Environment string
	// ChannelName is the username of the stream
	ChannelName string
	// BotUsername is the username of the bot
	BotUsername string
	// ExternalURL is the where the bot's HTTP server can be reached
	ExternalURL string
	// GoogleProjectID is the Google Cloud project ID
	GoogleProjectID string
	// GoogleMapsAPIKey is the API key with which we access Google Maps
	GoogleMapsAPIKey string
	// ReadOnly is used to prevent writing some things to the DB
	ReadOnly bool
	// Verbose determines output verbosity
	Verbose bool
	// DashcamDir contains the dashcam footage
	DashcamDir string
	// MapsOutputDir is where generated maps will be stored
	MapsOutputDir string
	// CroppedPath is where we store the cropped versions of screencaps (to OCR them)
	CroppedPath string
	// ScreencapDir is where we store full screenshots from the videos
	ScreencapDir string
	// VideoDir is where the videos live
	VideoDir string
	// DisableTwitchWebhooks disables recieving webhooks from Twitch (new followers for instance)
	DisableTwitchWebhooks bool
	// DisableMusicAutoplay disables the auto-play for MPD
	DisableMusicAutoplay bool

	// TripbotServerPort is used to specify the port on which the webserver runs
	TripbotServerPort string
	// TripbotServerPort is used to specify the port on which the VLC webserver runs
	VlcServerPort string
)

func init() {

	// set the Environment and load dotenv
	setEnvironment()

	requiredVars := []string{
		"CHANNEL_NAME",
		"BOT_USERNAME",
		"EXTERNAL_URL",
		"GOOGLE_APPS_PROJECT_ID",
		"GOOGLE_MAPS_API_KEY",
		"READ_ONLY",
		"DASHCAM_DIR",
		"MAPS_OUTPUT_DIR",
		"CROPPED_CORNERS_DIR",
		"TRIPBOT_SERVER_PORT",
		"VLC_SERVER_PORT",
	}
	for _, v := range requiredVars {
		_, ok := os.LookupEnv(v)
		if !ok {
			log.Fatalf("You must set %s", v)
		}
	}

	//TODO: consider using strings.ToLower() on channel name here and removing elsewhere
	ChannelName = os.Getenv("CHANNEL_NAME")
	BotUsername = os.Getenv("BOT_USERNAME")
	ExternalURL = os.Getenv("EXTERNAL_URL")
	GoogleProjectID = os.Getenv("GOOGLE_APPS_PROJECT_ID")
	GoogleMapsAPIKey = os.Getenv("GOOGLE_MAPS_API_KEY")
	ReadOnly, _ = strconv.ParseBool(os.Getenv("READ_ONLY"))
	Verbose, _ = strconv.ParseBool(os.Getenv("VERBOSE"))
	DashcamDir = os.Getenv("DASHCAM_DIR")
	MapsOutputDir = os.Getenv("MAPS_OUTPUT_DIR")
	CroppedPath = os.Getenv("CROPPED_CORNERS_DIR")

	VideoDir = path.Join(DashcamDir, videoDir)
	ScreencapDir = path.Join(DashcamDir, screencapDir)

	DisableTwitchWebhooks, _ = strconv.ParseBool(os.Getenv("DISABLE_TWITCH_WEBHOOKS"))
	DisableMusicAutoplay, _ = strconv.ParseBool(os.Getenv("DISABLE_MUSIC_AUTOPLAY"))

	TripbotServerPort = os.Getenv("TRIPBOT_SERVER_PORT")
	VlcServerPort = os.Getenv("VLC_SERVER_PORT")

	// check that the paths exist
	requiredDirs := []string{
		DashcamDir,
		VideoDir,
		ScreencapDir,
		CroppedPath,
		MapsOutputDir,
	}
	for _, d := range requiredDirs {
		// we cant use helpers.FileExists() here due to import loop
		_, err := os.Stat(d)
		if err != nil {
			if os.IsNotExist(err) {
				log.Fatalf("Directory %s does not exist", d)
			}
		}
	}
}

// setEnvironment sets the Environment var from the CLI
func setEnvironment() {
	var err error

	env, ok := os.LookupEnv("ENV")
	if !ok {
		log.Fatalln("You must set ENV")
	}

	// standardize the ENV
	switch env {
	case "stage", "staging":
		Environment = "staging"
	case "prod", "production":
		Environment = "production"
	case "dev", "development":
		Environment = "development"
	default:
		log.Fatalf("Unknown ENV: %s", env)
	}

	// load ENV vars from .env file
	err = godotenv.Load(".env." + Environment)

	if err != nil {
		log.Fatal("Error loading .env file:", err)
	}
}

//TODO: this should load from a config file
// IgnoredUsers are users who shouldn't be in the running for miles
// https://twitchinsights.net/bots
var IgnoredUsers = []string{
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
	"winsock",
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
	"!survey: Fill out a survey and help the stream",
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
