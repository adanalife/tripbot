package config

import (
	"log"
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

	// the run dir is created on boot if it isn't there; MkdirAll is a no-op
	// when it already is.
	if err := os.MkdirAll(cfg.RunDir, 0755); err != nil {
		log.Fatalf("Error creating directory %s: %s", cfg.RunDir, err)
	}
	return &cfg
}
