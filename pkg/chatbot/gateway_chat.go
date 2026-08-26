package chatbot

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/adanalife/tripbot/pkg/eventbus"
	"github.com/adanalife/tripbot/pkg/gateway"
	"github.com/adanalife/tripbot/pkg/instrumentation"
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

	// setChatConnected records whether the inbound chat transport is reachable.
	// nil (the default) reports nothing; ReportsChatConnection points it at the
	// chat-connection gauge. Distinct from setLive: this asks "can the bot
	// receive chat at all", which a failed poll answers (no), where it says
	// nothing about whether the channel is live.
	setChatConnected func(bool)
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

// LongPolls drops the floor under the gateway-suggested poll interval,
// returning the poller so it chains onto the constructor.
//
// Enable it against a gateway that holds an otherwise-empty inbound request open
// until a message arrives (Twitch, whose gateway terminates IRC). There the wait
// already happened inside the request, so sleeping afterwards would only add
// latency to every message. The gateway signals it by suggesting a 0 interval;
// the floor is what would otherwise override that.
//
// Safe against a gateway that doesn't long-poll: one that returns immediately
// also returns a non-zero suggested interval, and one too old to serve inbound
// chat at all errors, which backs off errWait.
func (p *gatewayChatPoller) LongPolls() *gatewayChatPoller {
	p.pollFloor = 0
	return p
}

// ReportsChatConnection makes the poller record whether it can reach the
// platform's inbound chat to the chat-connection gauge, returning the poller so
// it chains onto the constructor.
//
// Enable it where the gateway terminates a streaming chat transport (Twitch
// IRC), so "the bot is not in chat" stays observable even though the connection
// itself lives in the gateway. It reads the same page.Live flag as
// ReportsLiveness but answers a different question, so the two can both be on
// without fighting: for a platform whose chat exists only while it streams they
// happen to agree, and for Twitch — where chat is reachable off-stream —
// liveness comes from the OBS-side watchdog instead.
//
// A failed poll records 0 deliberately: whatever the cause (gateway down,
// gateway's own connection down, network in between), the bot is not receiving
// chat, which is exactly what the gauge means. Pairing it with the gateway's own
// platform_gateway_chat_connected localises a fault — gateway 1 / tripbot 0 puts
// it between them.
func (p *gatewayChatPoller) ReportsChatConnection() *gatewayChatPoller {
	p.setChatConnected = func(connected bool) {
		instrumentation.TwitchConnection.Set(connected)
	}
	return p
}

// NewGatewayChatPoller builds the production gateway-backed poller against the
// given platform-gateway base URL, feeding messages into this App's command
// path. Run it in a goroutine.
func (a *App) NewGatewayChatPoller(apiURL string) *gatewayChatPoller {
	return &gatewayChatPoller{
		client:     gateway.New(apiURL),
		handle:     a.HandleMessage,
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
			if p.setChatConnected != nil {
				p.setChatConnected(false)
			}
			if !sleepCtx(ctx, p.errWait) {
				return
			}
			continue
		}
		if p.setLive != nil {
			p.setLive(page.Live)
		}
		if p.setChatConnected != nil {
			p.setChatConnected(page.Live)
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
		p.handle(ctx, IncomingMessage{
			User:        m.Author,
			UserID:      m.AuthorID,
			Text:        m.Text,
			MessageID:   m.MessageID,
			Moderator:   m.Moderator,
			Subscriber:  m.Subscriber,
			Broadcaster: m.Broadcaster,
			Badges:      m.Badges,
			Emotes:      emotes(m.Emotes),
		})
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

// emotes retypes the gateway's emote occurrences as the event-bus shape. The
// two structs are identical field-for-field and deliberately separate: the
// event bus is a published contract the console generates types from, so it
// can't be pinned to whatever the gateway client happens to return.
func emotes(in []gateway.Emote) []eventbus.Emote {
	if len(in) == 0 {
		return nil
	}
	out := make([]eventbus.Emote, len(in))
	for i, e := range in {
		out[i] = eventbus.Emote{ID: e.ID, Start: e.Start, End: e.End}
	}
	return out
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
