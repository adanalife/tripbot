package config

import (
	"log"
	"log/slog"

	"github.com/adanalife/tripbot/pkg/config"
	"github.com/kelseyhightower/envconfig"
)

// Load reads the tripbot config from the environment — loading the
// env-specific dotenv file first — and returns it. main calls this once and
// passes the result into the constructors; nothing holds a package global.
func Load() *TripbotConfig {
	// set the Environment and load dotenv
	config.SetEnvironment()

	var cfg TripbotConfig
	if err := envconfig.Process("TRIPBOT", &cfg); err != nil {
		log.Fatalf("could not load config: %v", err)
	}

	//TODO: consider using strings.ToLower() on channel name here and removing elsewhere

	// give helpful reminders when things are disabled
	if cfg.GoogleMapsAPIKey == "" {
		slog.Warn("GOOGLE_MAPS_API_KEY not set — geocoder + static-map features disabled")
	}
	return &cfg
}
