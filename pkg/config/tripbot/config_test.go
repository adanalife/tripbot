package config

import (
	"os"
	"strconv"
	"testing"

	"github.com/adanalife/tripbot/pkg/contract"
)

// Load normalizes ChannelName so no caller has to lowercase it. This fails if a
// config path is ever added that skips the normalization.
func TestLoadLowercasesChannelName(t *testing.T) {
	t.Setenv("CHANNEL_NAME", "ADanaLife_")
	// godotenv doesn't overwrite an already-set var, so the value above wins
	// over .env.testing's CHANNEL_NAME.
	if got := os.Getenv("CHANNEL_NAME"); got != "ADanaLife_" {
		t.Fatalf("test setup: CHANNEL_NAME=%q", got)
	}

	if got := Load().ChannelName; got != "adanalife_" {
		t.Errorf("Load().ChannelName = %q, want %q", got, "adanalife_")
	}
}

// BotUsername sits one line from ChannelName in the same struct and has the
// same shape — a required platform login compared against values that are
// already lowercase. It keys the oauth_tokens lookup in particular, and those
// rows are written by the platform-gateway from Twitch's `login` field, which
// Twitch always returns lowercase. So a mixed-case BOT_USERNAME matches no row
// and the bot comes up with no token at all.
//
// The gateway fails loudly on the same input — its consent flow compares the
// signed-in login against its own BOT_USERNAME and returns an identity
// mismatch — which is exactly why tripbot's silent miss is worth pinning.
func TestLoadLowercasesBotUsername(t *testing.T) {
	t.Setenv("BOT_USERNAME", "TripBot4000")
	if got := os.Getenv("BOT_USERNAME"); got != "TripBot4000" {
		t.Fatalf("test setup: BOT_USERNAME=%q", got)
	}

	if got := Load().BotUsername; got != "tripbot4000" {
		t.Errorf("Load().BotUsername = %q, want %q", got, "tripbot4000")
	}
}

// The webserver's port is the contract's, not a literal spelled in a struct
// tag — a rename there has to reach the binary, which is the direction
// pkg/contract exists to hold. An explicit TRIPBOT_SERVER_PORT still wins.
func TestLoadPortDefaultsToTheContract(t *testing.T) {
	t.Setenv("TRIPBOT_SERVER_PORT", "")
	if got, want := Load().TripbotServerPort, strconv.Itoa(contract.PortTripbotHTTP); got != want {
		t.Errorf("Load().TripbotServerPort = %q, want %q", got, want)
	}

	t.Setenv("TRIPBOT_SERVER_PORT", "9999")
	if got := Load().TripbotServerPort; got != "9999" {
		t.Errorf("Load().TripbotServerPort = %q, want %q", got, "9999")
	}
}
