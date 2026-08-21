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

	// Platform logins are case-insensitive, so both are normalized once here —
	// callers compare them against lowercased usernames from the DB and never
	// have to lowercase them themselves. BotUsername in particular keys the
	// oauth_tokens lookup, whose rows the platform-gateway writes from Twitch's
	// `login` field, which is always lowercase: a mixed-case BOT_USERNAME would
	// match no row and the bot would come up with no token.
	cfg.ChannelName = strings.ToLower(cfg.ChannelName)
	cfg.BotUsername = strings.ToLower(cfg.BotUsername)

	// give helpful reminders when things are disabled
	if cfg.GoogleMapsAPIKey == "" {
		slog.Warn("GOOGLE_MAPS_API_KEY not set — geocoder + static-map features disabled")
	}
	return &cfg
}
