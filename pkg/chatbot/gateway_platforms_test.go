package chatbot

import (
	"testing"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
)

// Each Connect*ViaGateway has to install the client its platform can actually
// use: a posting client for youtube/facebook, a dropping one for
// tiktok/instagram. Getting that mapping backwards is silent — a tiktok
// instance would try to post to a nonexistent endpoint, and a youtube one
// would quietly stop answering — so it's pinned here.
func TestConnectViaGatewayInstallsThePlatformsClient(t *testing.T) {
	tests := []struct {
		platform   string
		connect    func(*App)
		wantPost   bool // gatewayChat (can post) vs noOutboundChat (drops)
		wantAPIURL string
	}{
		{platformYouTube, (*App).ConnectYouTubeViaGateway, true, "http://gw-youtube"},
		{platformFacebook, (*App).ConnectFacebookViaGateway, true, "http://gw-facebook"},
		{platformTikTok, (*App).ConnectTikTokViaGateway, false, ""},
		{platformInstagram, (*App).ConnectInstagramViaGateway, false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			a := &App{Cfg: &c.TripbotConfig{
				Environment:     "testing",
				ChannelName:     "test",
				BotUsername:     "test",
				Platform:        tt.platform,
				YouTubeAPIURL:   "http://gw-youtube",
				FacebookAPIURL:  "http://gw-facebook",
				TikTokAPIURL:    "http://gw-tiktok",
				InstagramAPIURL: "http://gw-instagram",
			}}
			tt.connect(a)

			// Every platform's outbound goes through the console mirror, or the
			// admin console stops seeing what the bot says.
			mirror, ok := a.Chat.(consoleMirror)
			if !ok {
				t.Fatalf("Chat = %T, want consoleMirror", a.Chat)
			}
			if mirror.platform != tt.platform {
				t.Errorf("mirror platform = %q, want %q", mirror.platform, tt.platform)
			}

			switch inner := mirror.inner.(type) {
			case gatewayChat:
				if !tt.wantPost {
					t.Fatalf("inner = gatewayChat, want noOutboundChat (%s cannot post)", tt.platform)
				}
				if inner.platform != tt.platform {
					t.Errorf("inner platform = %q, want %q", inner.platform, tt.platform)
				}
			case noOutboundChat:
				if tt.wantPost {
					t.Fatalf("inner = noOutboundChat, want gatewayChat (%s can post)", tt.platform)
				}
				if inner.platform != tt.platform {
					t.Errorf("inner platform = %q, want %q", inner.platform, tt.platform)
				}
			default:
				t.Fatalf("inner = %T, want gatewayChat or noOutboundChat", inner)
			}
		})
	}
}

// A platform that can't post must swallow Say rather than panic on a nil
// client — the commands call it unconditionally.
func TestNoOutboundChatDrops(t *testing.T) {
	cl := noOutboundChat{platform: platformTikTok}
	cl.Say("hello")
	cl.Whisper("someone", "hello")
}
