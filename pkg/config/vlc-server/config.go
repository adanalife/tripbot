package config

import (
	"log"
	"os"

	"github.com/adanalife/tripbot/pkg/config"
	"github.com/davecgh/go-spew/spew"
	"github.com/kelseyhightower/envconfig"
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
	config.SetEnvironment()

	Conf = LoadVlcServerConfig()

	spew.Dump(Conf)

	//TODO: consider using strings.ToLower() on channel name here and removing elsewhere

	// thes dirs will get created on boot if necessary
	dirsToCreate := []string{
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
		Conf.VideoDir,
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
