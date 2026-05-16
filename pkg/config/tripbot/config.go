package config

import (
	"log"
	"log/slog"

	"github.com/adanalife/tripbot/pkg/config"
	"github.com/kelseyhightower/envconfig"
)

var Conf *TripbotConfig

func LoadTripbotConfig() *TripbotConfig {
	var cfg TripbotConfig
	err := envconfig.Process("TRIPBOT", &cfg)
	if err != nil {
		log.Fatalf("could not load config: %v", err)
	}
	return &cfg
}

func init() {

	// set the Environment and load dotenv
	config.SetEnvironment()

	Conf = LoadTripbotConfig()

	//TODO: consider using strings.ToLower() on channel name here and removing elsewhere

	// give helpful reminders when things are disabled
	if Conf.DisableTwitchWebhooks {
		slog.Warn("Twitch webhooks disabled")
	}
}
