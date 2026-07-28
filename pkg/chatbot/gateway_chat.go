package chatbot

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	mylog "github.com/adanalife/tripbot/pkg/chatbot/log"
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
	client     inboundChatClient
	handle     func(ctx context.Context, msg IncomingMessage)
	handleGift func(ctx context.Context, gift IncomingGift)
	pollFloor  time.Duration // floor under the gateway-suggested interval
	errWait    time.Duration // backoff after a transport/gateway error
	platform   string        // stamped on the liveness gauge so instances don't collide
	// setLive records each page's Live flag. nil (the default) reports no
	// liveness at all; ReportsLiveness points it at the channel-live gauge.
	setLive func(bool)
}

// ReportsLiveness makes the poller record each page's Live flag to the
// channel-live gauge, returning the poller so it chains onto the constructor.
// Enable it only where the inbound poll is the instance's sole liveness source.
// A platform that also runs a broadcast-discovery tick has a second writer, and
// the two answers can disagree — YouTube's chat discovery reports not-live for
// an active broadcast whose chat is disabled, while broadcast discovery reports
// live — so two writers on one gauge would flap it, and the silent-disconnect
// alert with it.
func (p *gatewayChatPoller) ReportsLiveness() *gatewayChatPoller {
	platform := p.platform
	p.setLive = func(live bool) { instrumentation.ChannelLive.Set(live, platform) }
	return p
}

// NewGatewayChatPoller builds the production gateway-backed poller against the
// given platform-gateway base URL, feeding messages into this App's command
// path. Run it in a goroutine.
func (a *App) NewGatewayChatPoller(apiURL string) *gatewayChatPoller {
	return &gatewayChatPoller{
		client:     gateway.New(apiURL),
		handle:     a.HandleGatewayMessage,
		handleGift: a.HandleGatewayGift,
		pollFloor:  2 * time.Second,
		errWait:    time.Minute,
		platform:   a.Platform,
	}
}

// Run polls the gateway until ctx is done. The gateway returns an empty cursor
// when offline / chat ended, so forwarding it is the rediscover path;
// PollAfterMS carries the gateway's cadence (live interval, rediscover wait, or
// quota backoff). A transport/gateway error backs off errWait and retries.
//
// Under ReportsLiveness, each page's Live flag also feeds the channel-live
// gauge. The gateway resolves it from the platform itself — for TikTok, whether
// the webcast room is still there — so the poll doubles as the liveness signal
// at no extra platform call. A failed poll leaves the gauge alone: an
// unreachable gateway says nothing about whether the channel is live, and
// recording 0 there would read as a silent disconnect.
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
		if p.setLive != nil {
			p.setLive(page.Live)
		}
		cursor = page.Cursor
		for _, m := range page.Messages {
			p.route(ctx, m)
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

// route sends one inbound event to the handler for its kind. An unrecognized
// kind is dropped rather than treated as a comment: a newer gateway can add a
// kind this build has never heard of, and every non-comment kind so far has an
// empty Text, which would reach the command path as a blank line.
func (p *gatewayChatPoller) route(ctx context.Context, m gateway.InboundChatMessage) {
	switch m.Kind {
	case gateway.KindChat:
		p.handle(ctx, IncomingMessage{User: m.Author, UserID: m.AuthorID, Text: m.Text})
	case gateway.KindGift:
		if m.Gift == nil {
			slog.WarnContext(ctx, "gateway gift carries no gift payload", "author", m.Author)
			return
		}
		p.handleGift(ctx, IncomingGift{
			User:   m.Author,
			UserID: m.AuthorID,
			Name:   m.Gift.Name,
			Count:  m.Gift.Count,
			Value:  m.Gift.Value(),
		})
	default:
		slog.WarnContext(ctx, "unhandled gateway inbound kind; ignoring",
			"kind", string(m.Kind), "author", m.Author)
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
	mylog.ChatMsg(msg.User, a.Cfg.ChannelName, msg.Text)
	eventbus.EmitChatMessage(ctx, a.Cfg.Environment, a.Platform, msg.User, msg.Text)

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
