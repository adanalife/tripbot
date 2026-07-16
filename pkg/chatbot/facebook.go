package chatbot

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/adanalife/tripbot/pkg/gateway"
	"github.com/adanalife/tripbot/pkg/geo"
)

// gatewayFacebookChat is the outbound chat client for a facebook instance: it
// posts through gateway-facebook's SendChat, which resolves the Page's active
// live video itself and comments on it as the Page. It holds no video binding
// and no Page token — the gateway owns the send. Implements the
// provider-neutral ChatClient seam, mirroring gatewayYouTubeChat.
type gatewayFacebookChat struct {
	client *gateway.Client
}

func (g gatewayFacebookChat) Say(msg string) {
	// Twitch-only IRC emote command (the Chatter cron prefixes help messages
	// with it); on Facebook it would render as literal text.
	msg = strings.TrimPrefix(msg, "/me ")
	if err := g.client.SendChat(context.Background(), "", msg); err != nil {
		slog.Error("facebook gateway chat send failed", "err", err, "text", msg)
	}
}

func (gatewayFacebookChat) Whisper(username, msg string) {
	slog.Debug("facebook has no whispers; dropped", "to", username, "text", msg)
}

// ConnectFacebookViaGateway wires chat for a gateway-wired facebook instance
// (FACEBOOK_API_URL set). Both directions flow through gateway-facebook —
// outbound via SendChat here, inbound via NewGatewayChatPoller — so tripbot
// holds no Facebook credential at runtime.
func (a *App) ConnectFacebookViaGateway() {
	Uptime = time.Now()

	// process-wide geocoder warmup, same as ConnectYouTubeViaGateway / ConnectIRC.
	geo.SetDefault(geo.New(a.Cfg.GoogleMapsAPIKey))

	a.Chat = consoleMirror{
		inner:       gatewayFacebookChat{client: gateway.New(a.Cfg.FacebookAPIURL)},
		env:         a.Cfg.Environment,
		channel:     a.Cfg.ChannelName,
		platform:    a.Cfg.Platform,
		botUsername: a.Cfg.BotUsername,
	}
}
