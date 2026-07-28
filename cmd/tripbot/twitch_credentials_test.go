package main

import (
	"strings"
	"testing"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
)

// The credential requirement used to live in pkg/twitch's init(), so every
// platform's instance died without Twitch credentials it would never use.
// It's now scoped to the twitch bring-up — which means this check is the only
// thing standing between a credential-less twitch instance and a helix client
// it can't build.
func TestMissingTwitchCredentials(t *testing.T) {
	tests := []struct {
		name string
		cfg  *c.TripbotConfig
		want []string
	}{
		{
			name: "both set",
			cfg:  &c.TripbotConfig{TwitchClientID: "id", TwitchClientSecret: "secret"},
			want: nil,
		},
		{
			name: "neither set",
			cfg:  &c.TripbotConfig{},
			want: []string{"TWITCH_CLIENT_ID", "TWITCH_CLIENT_SECRET"},
		},
		{
			name: "id missing",
			cfg:  &c.TripbotConfig{TwitchClientSecret: "secret"},
			want: []string{"TWITCH_CLIENT_ID"},
		},
		{
			name: "secret missing",
			cfg:  &c.TripbotConfig{TwitchClientID: "id"},
			want: []string{"TWITCH_CLIENT_SECRET"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := missingTwitchCredentials(tt.cfg)
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("missing = %v, want %v", got, tt.want)
			}
		})
	}
}

// Only a twitch instance needs the credentials. Every other platform reaches
// its chat through a platform-gateway, so gating the check on the platform is
// what lets those instances drop the Twitch secret entirely.
func TestNonTwitchPlatformsSkipTheCredentialCheck(t *testing.T) {
	for _, platform := range []string{"tiktok", "instagram", "facebook", "youtube"} {
		t.Run(platform, func(t *testing.T) {
			bot := &Tripbot{cfg: &c.TripbotConfig{Platform: platform}}
			if bot.platformIsTwitch() {
				t.Fatalf("platformIsTwitch() = true for %q; it would demand Twitch credentials", platform)
			}
		})
	}

	// Empty PLATFORM is Twitch by contract, so it does need them.
	for _, platform := range []string{"", "twitch"} {
		bot := &Tripbot{cfg: &c.TripbotConfig{Platform: platform}}
		if !bot.platformIsTwitch() {
			t.Errorf("platformIsTwitch() = false for %q, want true", platform)
		}
	}
}
