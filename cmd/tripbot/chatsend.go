package main

import (
	"context"
	"encoding/json"
	"log/slog"

	chatEvents "github.com/adanalife/tripbot/pkg/chat-events"
	"github.com/adanalife/tripbot/pkg/chatsend"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/natsclient"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
	"github.com/nats-io/nats.go"
)

// startChatSendSubscriber wires the admin console's "send a chat message"
// command. The console (pkg/server) publishes chatEvents.Send on
// tripbot.<env>.chat.send; tripbot owns the Twitch identities, so it's the
// thing that actually sends. This keeps the console split-ready: post-split it
// publishes the same command and this subscriber relocates to whichever service
// ends up owning the Twitch tokens, with no change to the console or the wire.
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
	subject := chatEvents.SendSubject(c.Conf.Environment, chatEvents.PlatformTwitch)
	if _, err := conn.Subscribe(subject, func(m *nats.Msg) {
		var ev chatEvents.Send
		if err := json.Unmarshal(m.Data, &ev); err != nil {
			slog.ErrorContext(ctx, "chat.send: decode", "err", err, "subject", m.Subject)
			return
		}
		chatsend.Dispatch(ctx, ev,
			func(text string) { t.app.Chat.Say(text) },
			mytwitch.SendChatMessageAsBroadcaster,
		)
	}); err != nil {
		slog.ErrorContext(ctx, "chat.send subscribe failed", "err", err, "subject", subject)
		return
	}
	slog.InfoContext(ctx, "nats subscribed", "subject", subject)
}
