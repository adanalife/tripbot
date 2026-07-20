package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	chatEvents "github.com/adanalife/tripbot/pkg/chat-events"
	"github.com/adanalife/tripbot/pkg/chatsend"
	"github.com/adanalife/tripbot/pkg/gateway"
	"github.com/adanalife/tripbot/pkg/natsclient"
	"github.com/nats-io/nats.go"
)

// startChatSendSubscriber wires the "send a chat message" command. A publisher
// emits chatEvents.Send on tripbot.<env>.chat.send.<platform>; tripbot owns the
// Twitch identities, so it's the thing that actually sends. The in-tripbot admin
// panel that used to publish this was retired with the tripbot-console split;
// this subscriber stays as the receive side, ready for the standalone console to
// publish to over the same wire format when its chat-send feature lands.
//
// No-op when NATS is unconfigured (the singleton conn is nil) — the same
// fire-and-forget posture as the rest of the NATS surface. Must run after
// startNATS (conn) and setUpTwitchClient (which wires t.app.Chat via ConnectIRC).
func (t *Tripbot) startChatSendSubscriber(ctx context.Context) {
	conn := natsclient.Conn()
	if conn == nil {
		slog.InfoContext(ctx, "chat.send subscriber skipped (NATS_URL unset)")
		return
	}
	subject := chatEvents.SendSubject(t.cfg.Environment, chatEvents.PlatformTwitch)
	if _, err := conn.Subscribe(subject, func(m *nats.Msg) {
		var ev chatEvents.Send
		if err := json.Unmarshal(m.Data, &ev); err != nil {
			slog.ErrorContext(ctx, "chat.send: decode", "err", err, "subject", m.Subject)
			return
		}
		chatsend.Dispatch(ctx, ev,
			func(text string) { t.app.Chat.Say(text) },
			t.sendChatAsBroadcaster,
		)
	}); err != nil {
		slog.ErrorContext(ctx, "chat.send subscribe failed", "err", err, "subject", subject)
		return
	}
	slog.InfoContext(ctx, "nats subscribed", "subject", subject)
}

// sendChatAsBroadcaster posts text to the channel's chat as the broadcaster
// through the platform-gateway (the single Helix caller). Fail-open semantics
// are the caller's (chatsend.Dispatch logs and drops on error). Errors when no
// gateway is wired (a local/CI instance with no TWITCH_API_URL).
func (t *Tripbot) sendChatAsBroadcaster(ctx context.Context, text string) error {
	if t.gateway == nil {
		return errors.New("cannot send as broadcaster: no gateway configured (TWITCH_API_URL unset)")
	}
	return t.gateway.SendChat(ctx, gateway.IdentityBroadcaster, text)
}
