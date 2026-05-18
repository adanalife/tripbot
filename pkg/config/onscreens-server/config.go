package config

import (
	"log"
	"log/slog"
	"os"

	"github.com/adanalife/tripbot/pkg/config"
	"github.com/kelseyhightower/envconfig"
)

var Conf *OnscreensServerConfig

func LoadOnscreensServerConfig() *OnscreensServerConfig {
	var cfg OnscreensServerConfig
	err := envconfig.Process("ONSCREENS_SERVER", &cfg)
	if err != nil {
		log.Fatalf("could not load config: %v", err)
	}
	return &cfg
}

func init() {
	// set the Environment and load dotenv
	config.SetEnvironment()

	Conf = LoadOnscreensServerConfig()

	// these dirs will get created on boot if necessary
	dirsToCreate := []string{
		Conf.RunDir,
	}
	for _, d := range dirsToCreate {
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
}
