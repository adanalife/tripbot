package config

import (
	"log"
	"os"

	"github.com/davecgh/go-spew/spew"
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
	"github.com/logrusorgru/aurora"
)

var Conf *VlcServerConfig

func LoadVlcServerConfig() *VlcServerConfig {
	var cfg VlcServerConfig
	err := envconfig.Process("VLC_SERVER", &cfg)
	if err != nil {
		log.Fatalf("could not load config: %v", err)
	}
	return &cfg
}

func init() {

	// set the Environment and load dotenv
	setEnvironment()

	Conf = LoadVlcServerConfig()

	spew.Dump(Conf)

	//TODO: consider using strings.ToLower() on channel name here and removing elsewhere

	// // give helpful reminders when things are disabled
	if Conf.DisableTwitchWebhooks {
		log.Println(aurora.Yellow("Disabling Twitch webhooks"))
	}
	if Conf.DisableMusic {
		log.Println(aurora.Yellow("Disabling music"))
	}
	if Conf.DisableMusicAutoplay {
		log.Println(aurora.Yellow("Disabling music autoplay"))
	}

	// thes dirs will get created on boot if necessary
	dirsToCreate := []string{
		// Conf.MapsOutputDir,
		Conf.RunDir,
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
		Conf.DashcamDir,
		Conf.VideoDir,
		// Conf.MapsOutputDir,
		Conf.RunDir,
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
	var env string

	envVar, ok := os.LookupEnv("ENV")
	if !ok {
		log.Fatalln("You must set ENV")
	}

	// standardize the ENV
	switch envVar {
	case "stage", "staging":
		env = "staging"
	case "prod", "production":
		env = "production"
	case "dev", "development":
		env = "development"
	case "test", "testing":
		env = "testing"
	default:
		log.Fatalf("Unknown ENV: %s", envVar)
	}

	// load ENV vars from .env file
	err = godotenv.Load(".env." + env)

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
