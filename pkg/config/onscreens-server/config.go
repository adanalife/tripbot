package config

import (
	"fmt"
	"os"

	"github.com/adanalife/tripbot/pkg/config"
	"github.com/kelseyhightower/envconfig"
)

// Load reads the onscreens-server config from the environment — loading the
// env-specific dotenv file first — creates its run dir, and returns it. main
// calls this once and passes the result in; nothing holds a package global.
// Whether a failure here should end the process is main's call, so both
// failures come back as errors rather than exiting from inside the package.
func Load() (*OnscreensServerConfig, error) {
	// set the Environment and load dotenv
	config.SetEnvironment()

	var cfg OnscreensServerConfig
	if err := envconfig.Process("ONSCREENS_SERVER", &cfg); err != nil {
		return nil, fmt.Errorf("could not load config: %w", err)
	}

	// the run dir is created on boot if it isn't there; MkdirAll is a no-op
	// when it already is.
	if err := os.MkdirAll(cfg.RunDir, 0755); err != nil {
		return nil, fmt.Errorf("creating run dir %s: %w", cfg.RunDir, err)
	}
	return &cfg, nil
}
