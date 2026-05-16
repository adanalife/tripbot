package config

import (
	"log"
	"log/slog"
	"os"

	"github.com/adanalife/tripbot/pkg/config"
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

	//TODO: consider using strings.ToLower() on channel name here and removing elsewhere

	// these dirs will get created on boot if necessary
	dirsToCreate := []string{
		Conf.RunDir,
	}
	for _, d := range dirsToCreate {
		// we can't use helpers.FileExists() here due to import loop
		_, err := os.Stat(d)
		if err != nil {
			if os.IsNotExist(err) {
				slog.Info("creating directory", "dir", d)
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
		// we can't use helpers.FileExists() here due to import loop
		_, err := os.Stat(d)
		if err != nil {
			if os.IsNotExist(err) {
				log.Fatalf("Directory %s does not exist", d)
			}
		}
	}
}
