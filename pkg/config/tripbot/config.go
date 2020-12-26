package config

import (
	"log"
	"os"

	"github.com/adanalife/tripbot/pkg/config"
	"github.com/davecgh/go-spew/spew"
	"github.com/kelseyhightower/envconfig"
	"github.com/logrusorgru/aurora"
)

var Conf *TripbotConfig

func LoadTripbotConfig() *TripbotConfig {
	var cfg TripbotConfig
	err := envconfig.Process("TRIPBOT", &cfg)
	if err != nil {
		log.Fatalf("could not load config: %v", err)
	}
	return &cfg
}

func Initialize() {

	// set the Environment and load dotenv
	config.SetEnvironment()

	Conf = LoadTripbotConfig()

	spew.Dump(Conf)

	//TODO: consider using strings.ToLower() on channel name here and removing elsewhere

	// give helpful reminders when things are disabled
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
		Conf.ScreencapDir,
		// Conf.CroppedCornersDir,
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
		Conf.ScreencapDir,
		// Conf.VideoDir,
		// Conf.CroppedCornersDir,
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
