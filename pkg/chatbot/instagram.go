package chatbot

import (
	"log/slog"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/geo"
)

// instagramChat is the outbound ChatClient for an instagram instance. The
// Graph API can read live comments but not create them, so Say drops the
// message. Viewers still see command effects through the platform's own
// vlc/onscreens pipeline, and the consoleMirror wrapper keeps the would-be
// replies visible in the admin console — the same shape as tiktokChat.
type instagramChat struct{}

func (instagramChat) Say(msg string) {
	slog.Debug("instagram has no chat send; dropped", "text", msg)
}

func (instagramChat) Whisper(username, msg string) {
	slog.Debug("instagram has no whispers; dropped", "to", username, "text", msg)
}

// ConnectInstagramViaGateway wires chat for a gateway-wired instagram
// instance (INSTAGRAM_API_URL set). Only the inbound direction exists — the
// poll via NewGatewayChatPoller — so this just warms the geocoder and
// installs the drop-outbound client behind the console mirror.
func (a *App) ConnectInstagramViaGateway() {
	Uptime = time.Now()

	// process-wide geocoder warmup, same as ConnectYouTubeViaGateway / ConnectIRC.
	geo.SetDefault(geo.New(c.Conf.GoogleMapsAPIKey))

	a.Chat = consoleMirror{
		inner:       instagramChat{},
		env:         c.Conf.Environment,
		platform:    c.Conf.Platform,
		botUsername: c.Conf.BotUsername,
	}
}
