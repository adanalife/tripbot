package chatbot

import (
	"log/slog"
	"time"

	"github.com/adanalife/tripbot/pkg/geo"
)

// tiktokChat is the outbound ChatClient for a tiktok instance. TikTok has no
// chat-post API — the webcast protocol the gateway reads is observe-only — so
// Say drops the message. Viewers still see command effects through the
// platform's own playout/onscreens pipeline, and the consoleMirror wrapper keeps
// the would-be replies visible in the admin console.
type tiktokChat struct{}

func (tiktokChat) Say(msg string) {
	slog.Debug("tiktok has no chat send; dropped", "text", msg)
}

func (tiktokChat) Whisper(username, msg string) {
	slog.Debug("tiktok has no whispers; dropped", "to", username, "text", msg)
}

// ConnectTikTokViaGateway wires chat for a gateway-wired tiktok instance
// (TIKTOK_API_URL set). Only the inbound direction exists — the poll via
// NewGatewayChatPoller — so this just warms the geocoder and installs the
// drop-outbound client behind the console mirror.
func (a *App) ConnectTikTokViaGateway() {
	Uptime = time.Now()

	// process-wide geocoder warmup, same as ConnectYouTubeViaGateway / ConnectIRC.
	geo.SetDefault(geo.New(a.Cfg.GoogleMapsAPIKey))

	a.Chat = consoleMirror{
		inner:       tiktokChat{},
		env:         a.Cfg.Environment,
		channel:     a.Cfg.ChannelName,
		platform:    a.Cfg.Platform,
		botUsername: a.Cfg.BotUsername,
	}
}
