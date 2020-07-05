package config

import (
	"log"
	"os"
	"path"
	"strconv"

	"github.com/joho/godotenv"
	"github.com/logrusorgru/aurora"
)

//TODO: not all required ENV vars are required for vlc-server

const (
	// these are the default subdirectories
	videoDir = "_all"

	DBPath            = "db/tripbot.db"
	UserJoinsBucket   = "user_joins"
	UserWatchedBucket = "user_watched"
	CoordsBucket      = "coords"
)

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
	// CroppedCornersDir is where we store the cropped versions of screencaps (to OCR them)
	CroppedCornersDir string
	// ScreencapDir is where we store full screenshots from the videos
	ScreencapDir string
	// VideoDir is where the videos live
	VideoDir string
	RunDir   string

	// DisableTwitchWebhooks disables receiving webhooks from Twitch (new followers for instance)
	DisableTwitchWebhooks bool
	// DisableMusicAutoplay disables the auto-play for MPD
	DisableMusicAutoplay bool

	// TripbotHttpAuth is used to authenticate users to the HTTP server
	TripbotHttpAuth string
	// TripbotServerPort is used to specify the port on which the webserver runs
	TripbotServerPort string
	// VlcServerHost is used to specify the host for the VLC webserver
	VlcServerHost string
	MpdServerHost string
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
		"SCREENCAP_DIR",
		"MAPS_OUTPUT_DIR",
		"CROPPED_CORNERS_DIR",
		"RUN_DIR",
		"TRIPBOT_HTTP_AUTH",
		"TRIPBOT_SERVER_PORT",
		"VLC_SERVER_HOST",
		"MPD_SERVER_HOST",
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
	ReadOnly, _ = strconv.ParseBool(os.Getenv("READ_ONLY"))
	Verbose, _ = strconv.ParseBool(os.Getenv("VERBOSE"))

	// directory settings
	DashcamDir = os.Getenv("DASHCAM_DIR")
	ScreencapDir = os.Getenv("SCREENCAP_DIR")
	MapsOutputDir = os.Getenv("MAPS_OUTPUT_DIR")
	CroppedCornersDir = os.Getenv("CROPPED_CORNERS_DIR")
	RunDir = os.Getenv("RUN_DIR")

	// HTTP server settings
	ExternalURL = os.Getenv("EXTERNAL_URL")
	TripbotHttpAuth = os.Getenv("TRIPBOT_HTTP_AUTH")
	TripbotServerPort = os.Getenv("TRIPBOT_SERVER_PORT")
	VlcServerHost = os.Getenv("VLC_SERVER_HOST")
	MpdServerHost = os.Getenv("MPD_SERVER_HOST")

	// google-specific settings
	GoogleProjectID = os.Getenv("GOOGLE_APPS_PROJECT_ID")
	GoogleMapsAPIKey = os.Getenv("GOOGLE_MAPS_API_KEY")

	DisableTwitchWebhooks, _ = strconv.ParseBool(os.Getenv("DISABLE_TWITCH_WEBHOOKS"))
	DisableMusicAutoplay, _ = strconv.ParseBool(os.Getenv("DISABLE_MUSIC_AUTOPLAY"))

	// give helpful reminders when things are disabled
	if DisableTwitchWebhooks {
		log.Println(aurora.Yellow("Disabling Twitch webhooks"))
	}
	if DisableMusicAutoplay {
		log.Println(aurora.Yellow("Disabling music autoplay"))
	}

	// assemble compound settings
	VideoDir = path.Join(DashcamDir, videoDir)

	// thes dirs will get created on boot if necessary
	dirsToCreate := []string{
		ScreencapDir,
		CroppedCornersDir,
		MapsOutputDir,
		RunDir,
	}
	for _, d := range dirsToCreate {
		// we cant use helpers.FileExists() here due to import loop
		_, err := os.Stat(d)
		if err != nil {
			if os.IsNotExist(err) {
				log.Println("Creating directory", d)
				err = os.MkdirAll(d, 0755)
				if err != nil {
					log.Fatalf("Error creating directory %s: %s", d, err)
				}
			}
		}
	}

	// check that the paths exist
	requiredDirs := []string{
		DashcamDir,
		VideoDir,
		ScreencapDir,
		CroppedCornersDir,
		MapsOutputDir,
		RunDir,
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
	case "test", "testing":
		Environment = "testing"
	default:
		log.Fatalf("Unknown ENV: %s", env)
	}

	// load ENV vars from .env file
	err = godotenv.Load(".env." + Environment)

	if err != nil {
		log.Println("Error loading .env file:", err)
		log.Println("Continuing anyway...")
	}
}

//TODO: this should load from a config file
// IgnoredUsers are users who shouldn't be in the running for miles
// https://twitchinsights.net/bots
var IgnoredUsers = []string{
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
	"mathgaming",
	"mrreflector",
	"n3td3v",
	"nightbot",
	"p0lizei_",
	"slocool",
	"streamlabs",
	"taormina2600",
	"teyyd",
	"tripbot4000",
	"unixchat",
	"v_and_k",
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
	// "!song: Get the current music",
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
