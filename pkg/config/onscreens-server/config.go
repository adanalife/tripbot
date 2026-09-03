package config

import (
	"fmt"

	"github.com/adanalife/tripbot/pkg/config"
	"github.com/kelseyhightower/envconfig"
)

// Load reads the onscreens-server config from the environment — loading the
// env-specific dotenv file first — and returns it. main calls this once and
// passes the result in; nothing holds a package global. Whether a failure here
// should end the process is main's call, so it comes back as an error rather
// than exiting from inside the package.
func Load() (*OnscreensServerConfig, error) {
	// set the Environment and load dotenv
	config.SetEnvironment()

	var cfg OnscreensServerConfig
	if err := envconfig.Process("ONSCREENS_SERVER", &cfg); err != nil {
		return nil, fmt.Errorf("could not load config: %w", err)
	}
	return &cfg, nil
}
