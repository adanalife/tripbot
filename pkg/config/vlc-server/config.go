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

	// give helpful reminders when things are disabled
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
