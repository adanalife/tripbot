package config

import (
	"log"
	"log/slog"
	"os"

	"github.com/adanalife/tripbot/pkg/config"
	"github.com/kelseyhightower/envconfig"
)

// Load reads the onscreens-server config from the environment — loading the
// env-specific dotenv file first — creates its run dir, and returns it. main
// calls this once and passes the result in; nothing holds a package global.
func Load() *OnscreensServerConfig {
	// set the Environment and load dotenv
	config.SetEnvironment()

	var cfg OnscreensServerConfig
	if err := envconfig.Process("ONSCREENS_SERVER", &cfg); err != nil {
		log.Fatalf("could not load config: %v", err)
	}

	// these dirs will get created on boot if necessary
	dirsToCreate := []string{
		cfg.RunDir,
	}
	for _, d := range dirsToCreate {
		if _, err := os.Stat(d); err != nil {
			if os.IsNotExist(err) {
				slog.Info("creating directory", "dir", d)
				if err := os.MkdirAll(d, 0755); err != nil {
					log.Fatalf("Error creating directory %s: %s", d, err)
				}
			}
		}
	}
	return &cfg
}
