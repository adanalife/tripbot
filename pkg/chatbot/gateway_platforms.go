package chatbot

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/adanalife/tripbot/pkg/gateway"
	"github.com/adanalife/tripbot/pkg/geo"
)

// The gateway-wired platforms (everything except Twitch) split two ways on
// outbound chat, and that split is the only thing that differs between them:
//
//   - youtube, facebook — the gateway can post, so Say goes out through its
//     SendChat. The gateway owns the credential and resolves the target itself
//     (YouTube's active live chat, Facebook's live video), so tripbot holds no
//     platform token and binds no chat ID.
//   - tiktok, instagram — the platform has no way to post. TikTok's webcast
//     protocol is observe-only, and Instagram's Graph API can read live
//     comments but not create them. Say drops the message; viewers still see
//     command effects through playout/onscreens, and consoleMirror keeps the
//     would-be reply visible in the admin console.
//
// Inbound is uniform across all four — NewGatewayChatPoller against the same
// gateway — so it lives in gateway_chat.go rather than here.

// gatewayChat is the outbound ChatClient for a platform whose gateway can post.
// platform names it in log lines. Implements the provider-neutral ChatClient
// seam.
type gatewayChat struct {
	client   *gateway.Client
	platform string
}

func (g gatewayChat) Say(msg string) {
	// Twitch-only IRC emote command (the Chatter cron prefixes help messages
	// with it); anywhere else it would render as literal text.
	msg = strings.TrimPrefix(msg, "/me ")
	if err := g.client.SendChat(context.Background(), "", msg); err != nil {
		slog.Error(g.platform+" gateway chat send failed", "err", err, "text", msg)
	}
}

// Whisper drops: no gateway platform has a whisper equivalent.
func (g gatewayChat) Whisper(username, msg string) {
	slog.Debug(g.platform+" has no whispers; dropped", "to", username, "text", msg)
}

// noOutboundChat is the ChatClient for a platform tripbot can read but not
// write. Both methods drop with a debug line.
type noOutboundChat struct {
	platform string
}

func (c noOutboundChat) Say(msg string) {
	slog.Debug(c.platform+" has no chat send; dropped", "text", msg)
}

func (c noOutboundChat) Whisper(username, msg string) {
	slog.Debug(c.platform+" has no whispers; dropped", "to", username, "text", msg)
}

// connectViaGateway installs inner as the App's outbound chat client behind the
// console mirror and warms the process-wide geocoder, the same way ConnectIRC
// does for Twitch. There is no connection to fail: the gateway holds the
// credential, and inbound runs as a separate poller.
func (a *App) connectViaGateway(inner ChatClient) {
	Uptime = time.Now()

	geo.SetDefault(geo.New(a.Cfg.GoogleMapsAPIKey))

	a.Chat = consoleMirror{
		inner:       inner,
		env:         a.Cfg.Environment,
		channel:     a.Cfg.ChannelName,
		platform:    a.Cfg.Platform,
		botUsername: a.Cfg.BotUsername,
	}
}

// ConnectYouTubeViaGateway wires a youtube instance (YOUTUBE_API_URL set).
// Both directions flow through gateway-youtube.
func (a *App) ConnectYouTubeViaGateway() {
	a.connectViaGateway(gatewayChat{
		client:   gateway.New(a.Cfg.YouTubeAPIURL),
		platform: platformYouTube,
	})
}

// ConnectFacebookViaGateway wires a facebook instance (FACEBOOK_API_URL set).
// Both directions flow through gateway-facebook; outbound lands as a Page
// comment on the live video.
func (a *App) ConnectFacebookViaGateway() {
	a.connectViaGateway(gatewayChat{
		client:   gateway.New(a.Cfg.FacebookAPIURL),
		platform: platformFacebook,
	})
}

// ConnectTikTokViaGateway wires a tiktok instance (TIKTOK_API_URL set).
// Inbound only — TikTok has no chat-post API.
func (a *App) ConnectTikTokViaGateway() {
	a.connectViaGateway(noOutboundChat{platform: platformTikTok})
}

// ConnectInstagramViaGateway wires an instagram instance (INSTAGRAM_API_URL
// set). Inbound only — the Graph API cannot create live comments.
func (a *App) ConnectInstagramViaGateway() {
	a.connectViaGateway(noOutboundChat{platform: platformInstagram})
}
