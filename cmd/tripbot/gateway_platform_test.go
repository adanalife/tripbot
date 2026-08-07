package main

import (
	"testing"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
)

func testCfg(platform string) *c.TripbotConfig {
	return &c.TripbotConfig{
		Platform:              platform,
		YouTubeAPIURL:         "http://gw-youtube",
		FacebookAPIURL:        "http://gw-facebook",
		InstagramAPIURL:       "http://gw-instagram",
		TikTokAPIURL:          "http://gw-tiktok",
		YouTubeInboundEnabled: true,
	}
}

// Every non-Twitch platform's chat bring-up is driven off this descriptor, so a
// wrong field here is a silently mis-wired instance: the wrong gateway URL, an
// error naming the wrong env var, or — the one with teeth — tiktok losing
// reportsLiveness, which is its only signal that the LIVE room is still up.
func TestGatewayPlatform(t *testing.T) {
	tests := []struct {
		platform        string
		wantName        string
		wantEnvVar      string
		wantAPIURL      string
		wantDirections  string
		reportsLiveness bool
	}{
		{"facebook", "facebook", "FACEBOOK_API_URL", "http://gw-facebook", "inbound + outbound", false},
		{"instagram", "instagram", "INSTAGRAM_API_URL", "http://gw-instagram", "inbound only", false},
		{"tiktok", "tiktok", "TIKTOK_API_URL", "http://gw-tiktok", "inbound only", true},
		{"youtube", "youtube", "YOUTUBE_API_URL", "http://gw-youtube", "inbound + outbound", false},
		// An unrecognized platform falls through to youtube, as the if/else
		// dispatch this replaced did.
		{"peertube", "youtube", "YOUTUBE_API_URL", "http://gw-youtube", "inbound + outbound", false},
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			p := (&Tripbot{cfg: testCfg(tt.platform)}).gatewayPlatform()
			if p.name != tt.wantName {
				t.Errorf("name = %q, want %q", p.name, tt.wantName)
			}
			if p.envVar != tt.wantEnvVar {
				t.Errorf("envVar = %q, want %q", p.envVar, tt.wantEnvVar)
			}
			if p.apiURL != tt.wantAPIURL {
				t.Errorf("apiURL = %q, want %q", p.apiURL, tt.wantAPIURL)
			}
			if p.directions != tt.wantDirections {
				t.Errorf("directions = %q, want %q", p.directions, tt.wantDirections)
			}
			if p.reportsLiveness != tt.reportsLiveness {
				t.Errorf("reportsLiveness = %v, want %v", p.reportsLiveness, tt.reportsLiveness)
			}
			if p.connect == nil {
				t.Error("connect is nil; no outbound client would be installed")
			}
		})
	}
}

// Only YouTube gates its inbound poll, and it's the expensive one — a
// skipInbound that inverted would either burn the Data API quota the bot-less
// mode exists to save, or silently stop reading chat.
func TestGatewayPlatformYouTubeInboundGate(t *testing.T) {
	cfg := testCfg("youtube")
	cfg.YouTubeInboundEnabled = false
	off := (&Tripbot{cfg: cfg}).gatewayPlatform()
	if !off.skipInbound {
		t.Error("skipInbound = false with YOUTUBE_INBOUND_ENABLED=false, want true")
	}
	if off.inboundOffReason == "" || off.inboundOffFix == "" {
		t.Error("bot-less mode logs nothing explaining why chat is silent")
	}

	on := (&Tripbot{cfg: testCfg("youtube")}).gatewayPlatform()
	if on.skipInbound {
		t.Error("skipInbound = true with YOUTUBE_INBOUND_ENABLED=true, want false")
	}

	// No other platform gates inbound; a stray skipInbound would leave that
	// platform's chat unread with no obvious cause.
	for _, platform := range []string{"facebook", "instagram", "tiktok"} {
		if (&Tripbot{cfg: testCfg(platform)}).gatewayPlatform().skipInbound {
			t.Errorf("%s skipInbound = true, want false", platform)
		}
	}
}
