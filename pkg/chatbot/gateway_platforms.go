package chatbot

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/adanalife/tripbot/pkg/gateway"
	"github.com/adanalife/tripbot/pkg/geo"
)

// Every platform reaches chat through its platform-gateway, and they split two
// ways on outbound:
//
//   - twitch, youtube, facebook — the gateway can post, so Say goes out through
//     its SendChat. The gateway owns the credential and resolves the target
//     itself (Twitch's IRC connection, YouTube's active live chat, Facebook's
//     live video), so tripbot holds no platform token and binds no chat ID.
//   - tiktok, instagram — the platform has no way to post. TikTok's webcast
//     protocol is observe-only, and Instagram's Graph API can read live
//     comments but not create them. Say drops the message; viewers still see
//     command effects through playout/onscreens, and consoleMirror keeps the
//     would-be reply visible in the admin console.
//
// Inbound is uniform across all of them — NewGatewayChatPoller against the same
// gateway — so it lives in gateway_chat.go rather than here.

// gatewayChat is the outbound ChatClient for a platform whose gateway can post.
// platform names it in log lines. Implements the provider-neutral ChatClient
// seam.
type gatewayChat struct {
	client   *gateway.Client
	platform string

	// identity selects which of the platform's credentials speaks. Empty lets
	// the gateway pick its default, which is what the single-identity platforms
	// want; Twitch has both a bot and a broadcaster identity, so it names one.
	identity string

	// keepActions leaves a leading "/me " in place. Only Twitch understands it
	// as an action command — and only because its gateway sends over IRC, where
	// chat commands are interpreted. Everywhere else it would post as literal
	// text, so it's stripped.
	keepActions bool
}

func (g gatewayChat) Say(msg string) {
	if !g.keepActions {
		msg = strings.TrimPrefix(msg, "/me ")
	}
	if err := g.client.SendChat(context.Background(), g.identity, msg); err != nil {
		slog.Error(g.platform+" gateway chat send failed", "err", err, "text", msg)
	}
}

// noOutboundChat is the ChatClient for a platform tripbot can read but not
// write. Say drops with a debug line.
type noOutboundChat struct {
	platform string
}

func (c noOutboundChat) Say(msg string) {
	slog.Debug(c.platform+" has no chat send; dropped", "text", msg)
}

// connectViaGateway installs inner as the App's outbound chat client behind the
// console mirror and warms the process-wide geocoder. There is no connection to
// fail: the gateway holds the credential, and inbound runs as a separate poller.
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

// ConnectTwitchViaGateway wires a twitch instance (TWITCH_API_URL set). Both
// directions flow through gateway-twitch, which terminates the channel's IRC
// connection: outbound sends as the bot identity over that same connection, and
// inbound arrives on the shared cursored poll.
//
// The bot identity is named explicitly because Twitch is the one platform with
// two credentials — the gateway's default is the broadcaster, which is who the
// console's send-as-broadcaster path wants, not who the bot speaks as.
func (a *App) ConnectTwitchViaGateway() {
	a.connectViaGateway(gatewayChat{
		client:      gateway.New(a.Cfg.TwitchAPIURL),
		platform:    platformTwitch,
		identity:    gateway.IdentityBot,
		keepActions: true,
	})
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
