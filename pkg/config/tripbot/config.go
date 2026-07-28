package config

import (
	"log"
	"log/slog"
	"strings"

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

	// Platform logins are case-insensitive, so the channel name is normalized
	// once here — callers compare it against lowercased usernames from the DB
	// and never have to lowercase it themselves.
	cfg.ChannelName = strings.ToLower(cfg.ChannelName)

	// give helpful reminders when things are disabled
	if cfg.GoogleMapsAPIKey == "" {
		slog.Warn("GOOGLE_MAPS_API_KEY not set — geocoder + static-map features disabled")
	}
	return &cfg
}
