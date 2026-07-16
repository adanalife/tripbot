package chatbot

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/adanalife/tripbot/pkg/gateway"
	"github.com/adanalife/tripbot/pkg/geo"
)

// gatewayYouTubeChat is the outbound chat client for a youtube instance: it
// posts through gateway-youtube's SendChat, which resolves the channel's active
// live chat itself. It holds no live-chat binding and no YouTube token — the
// gateway owns the send and the chat-ID resolution. Implements the
// provider-neutral ChatClient seam.
type gatewayYouTubeChat struct {
	client *gateway.Client
}

func (g gatewayYouTubeChat) Say(msg string) {
	// Twitch-only IRC emote command (the Chatter cron prefixes help messages
	// with it); on YouTube it would render as literal text.
	msg = strings.TrimPrefix(msg, "/me ")
	if err := g.client.SendChat(context.Background(), "", msg); err != nil {
		slog.Error("youtube gateway chat send failed", "err", err, "text", msg)
	}
}

func (gatewayYouTubeChat) Whisper(username, msg string) {
	slog.Debug("youtube has no whispers; dropped", "to", username, "text", msg)
}

// ConnectYouTubeViaGateway wires outbound chat for a gateway-wired youtube
// instance (YOUTUBE_API_URL set). Both directions flow through gateway-youtube —
// outbound via SendChat here, inbound via NewGatewayChatPoller — so tripbot
// holds no YouTube token at runtime. Unlike ConnectYouTube it reads no
// oauth_tokens row and binds no live chat, so there's nothing to fail on.
func (a *App) ConnectYouTubeViaGateway() {
	Uptime = time.Now()

	// process-wide geocoder warmup, same as ConnectYouTube / ConnectIRC.
	geo.SetDefault(geo.New(a.Cfg.GoogleMapsAPIKey))

	a.Chat = consoleMirror{
		inner:       gatewayYouTubeChat{client: gateway.New(a.Cfg.YouTubeAPIURL)},
		env:         a.Cfg.Environment,
		channel:     a.Cfg.ChannelName,
		platform:    a.Cfg.Platform,
		botUsername: a.Cfg.BotUsername,
	}
}
