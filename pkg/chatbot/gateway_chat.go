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
	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/adanalife/tripbot/pkg/users"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// inboundChatClient is the subset of *gateway.Client the gateway poller needs;
// a fake satisfies it in tests.
type inboundChatClient interface {
	InboundChat(ctx context.Context, cursor string) (gateway.InboundChatPage, error)
}

// gatewayChatPoller is the inbound transport for a gateway-wired platform
// instance (youtube, tiktok): it drives the platform gateway's
// GET /v1/chat/inbound, feeding each returned viewer message into the shared
// command path. The gateway owns discovery, paging, backlog-skip, own-echo
// filtering, and the poll cadence, so this loop just advances the opaque
// cursor and sleeps the suggested interval — holding no platform credential.
type gatewayChatPoller struct {
	client    inboundChatClient
	handle    func(ctx context.Context, msg IncomingMessage)
	pollFloor time.Duration // floor under the gateway-suggested interval
	errWait   time.Duration // backoff after a transport/gateway error
}

// NewGatewayChatPoller builds the production gateway-backed poller against the
// given platform-gateway base URL, feeding messages into this App's command
// path. Run it in a goroutine.
func (a *App) NewGatewayChatPoller(apiURL string) *gatewayChatPoller {
	return &gatewayChatPoller{
		client:    gateway.New(apiURL),
		handle:    a.HandleGatewayMessage,
		pollFloor: 2 * time.Second,
		errWait:   time.Minute,
	}
}

// Run polls the gateway until ctx is done. The gateway returns an empty cursor
// when offline / chat ended, so forwarding it is the rediscover path;
// PollAfterMS carries the gateway's cadence (live interval, rediscover wait, or
// quota backoff). A transport/gateway error backs off errWait and retries.
func (p *gatewayChatPoller) Run(ctx context.Context) {
	cursor := ""
	for ctx.Err() == nil {
		page, err := p.client.InboundChat(ctx, cursor)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			slog.ErrorContext(ctx, "gateway inbound poll failed", "err", err)
			if !sleepCtx(ctx, p.errWait) {
				return
			}
			continue
		}
		cursor = page.Cursor
		for _, m := range page.Messages {
			p.handle(ctx, IncomingMessage{User: m.Author, UserID: m.AuthorID, Text: m.Text})
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

// HandleGatewayMessage processes one inbound chat message from a gateway-wired
// platform. Identical to HandleMessage except the login step: gateway-platform
// viewers are NOT logged in or persisted — v1 punts identity, presence, and
// miles entirely (see the v1 command allowlist), so the command path gets a
// transient User carrying just the display name. The Loki chat line, the
// admin-console event-bus mirror, and the metrics all stay.
func (a *App) HandleGatewayMessage(ctx context.Context, msg IncomingMessage) {
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
