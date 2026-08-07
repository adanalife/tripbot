package config

import (
	"os"
	"testing"
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
