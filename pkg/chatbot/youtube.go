package chatbot

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	mylog "github.com/adanalife/tripbot/pkg/chatbot/log"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/eventbus"
	"github.com/adanalife/tripbot/pkg/gateway"
	"github.com/adanalife/tripbot/pkg/geo"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/adanalife/tripbot/pkg/users"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
// outbound via SendChat here, inbound via NewGatewayYouTubeChatPoller — so
// tripbot holds no YouTube token at runtime. Unlike ConnectYouTube it reads no
// oauth_tokens row and binds no live chat, so there's nothing to fail on.
func (a *App) ConnectYouTubeViaGateway() {
	Uptime = time.Now()

	// process-wide geocoder warmup, same as ConnectYouTube / ConnectIRC.
	geo.SetDefault(geo.New(c.Conf.GoogleMapsAPIKey))

	a.Chat = consoleMirror{
		inner:       gatewayYouTubeChat{client: gateway.New(c.Conf.YouTubeAPIURL)},
		env:         c.Conf.Environment,
		platform:    c.Conf.Platform,
		botUsername: c.Conf.BotUsername,
	}
}

// inboundChatClient is the subset of *gateway.Client the gateway poller needs;
// a fake satisfies it in tests.
type inboundChatClient interface {
	InboundChat(ctx context.Context, cursor string) (gateway.InboundChatPage, error)
}

// gatewayYouTubeChatPoller is the inbound transport for a gateway-wired youtube
// instance: it drives gateway-youtube's GET /v1/chat/inbound, feeding each
// returned viewer message into the shared command path. The gateway owns
// discovery, paging, backlog-skip, own-echo filtering, and the poll cadence, so
// this loop just advances the opaque cursor and sleeps the suggested interval —
// holding no YouTube token. The gateway analog of youtubeChatPoller.
type gatewayYouTubeChatPoller struct {
	client    inboundChatClient
	handle    func(ctx context.Context, msg IncomingMessage)
	pollFloor time.Duration // floor under the gateway-suggested interval
	errWait   time.Duration // backoff after a transport/gateway error
}

// NewGatewayYouTubeChatPoller builds the production gateway-backed poller,
// feeding messages into this App's command path. Run it in a goroutine.
func (a *App) NewGatewayYouTubeChatPoller() *gatewayYouTubeChatPoller {
	return &gatewayYouTubeChatPoller{
		client:    gateway.New(c.Conf.YouTubeAPIURL),
		handle:    a.HandleYouTubeMessage,
		pollFloor: 2 * time.Second,
		errWait:   time.Minute,
	}
}

// Run polls the gateway until ctx is done. The gateway returns an empty cursor
// when offline / chat ended, so forwarding it is the rediscover path;
// PollAfterMS carries the gateway's cadence (live interval, rediscover wait, or
// quota backoff). A transport/gateway error backs off errWait and retries.
func (p *gatewayYouTubeChatPoller) Run(ctx context.Context) {
	cursor := ""
	for ctx.Err() == nil {
		page, err := p.client.InboundChat(ctx, cursor)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			slog.ErrorContext(ctx, "youtube gateway inbound poll failed", "err", err)
			if !sleepCtx(ctx, p.errWait) {
				return
			}
			continue
		}
		cursor = page.Cursor
		for _, m := range page.Messages {
			p.handle(ctx, IncomingMessage{User: m.Author, Text: m.Text})
		}
		wait := time.Duration(page.PollAfterMS) * time.Millisecond
		if wait < p.pollFloor {
			wait = p.pollFloor
		}
		if !sleepCtx(ctx, wait) {
			return
		}
	}
}

// HandleYouTubeMessage processes one inbound YouTube chat message. Identical
// to HandleMessage except the login step: YouTube viewers are NOT logged in
// or persisted — v1 punts identity, presence, and miles entirely (see the
// platform allowlist), so the command path gets a transient User carrying
// just the display name. The Loki chat line, the admin-console event-bus
// mirror, and the metrics all stay.
func (a *App) HandleYouTubeMessage(ctx context.Context, msg IncomingMessage) {
	// span attribute key shared with the Twitch path for observability
	// continuity; renaming both to a platform-tagged key is the B4 pass.
	ctx, span := tracer.Start(ctx, "chatbot.handle_message",
		trace.WithAttributes(attribute.String("twitch.user", msg.User)))
	defer span.End()

	instrumentation.ChatMessages.Inc()
	mylog.ChatMsg(msg.User, msg.Text)
	eventbus.EmitChatMessage(ctx, c.Conf.Environment, a.Platform, msg.User, msg.Text)

	// transient, never written to the users table — the allowlisted command
	// subset reads nothing user-specific beyond the name.
	user := &users.User{Username: strings.ToLower(msg.User)}
	a.runCommand(ctx, user, strings.ToLower(msg.Text))
}

// sleepCtx waits d or until ctx is done; false means ctx ended first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
