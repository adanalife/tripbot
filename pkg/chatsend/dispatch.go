// Package chatsend holds the pure routing for the chat.send command: given a
// decoded chatEvents.Send, call the right sender. It's split
// out of cmd/tripbot's NATS subscriber so the routing is unit-testable —
// cmd/tripbot (package main) imports github.com/dimiro1/banner/autoload, whose
// init() runs flag.Parse() and makes the package untestable under `go test`.
//
// The senders are injected as plain funcs so this package stays free of the
// Twitch/IRC machinery: cmd/tripbot passes the bot's Say and the broadcaster
// Helix send; tests pass fakes.
package chatsend

import (
	"context"
	"log/slog"

	chatEvents "github.com/adanalife/tripbot/pkg/chat-events"
)

// Dispatch routes a decoded Send to the right sender. botSay is the bot's IRC
// Say (which mirrors onto the live console); broadcasterSay is the Helix
// "send as broadcaster" call (whose result the bot reads back inbound,
// surfacing on the console via the normal chat.message path). An empty text or
// unknown identity is dropped with a warning — a publisher bug, not an intent.
func Dispatch(
	ctx context.Context,
	ev chatEvents.Send,
	botSay func(text string),
	broadcasterSay func(ctx context.Context, text string) error,
) {
	if ev.Text == "" {
		slog.WarnContext(ctx, "chat.send: empty text dropped", "identity", ev.Identity)
		return
	}
	switch ev.Identity {
	case chatEvents.IdentityBot:
		botSay(ev.Text)
		slog.InfoContext(ctx, "chat.send: sent", "identity", ev.Identity, "text", ev.Text)
	case chatEvents.IdentityBroadcaster:
		if err := broadcasterSay(ctx, ev.Text); err != nil {
			slog.ErrorContext(ctx, "chat.send: broadcaster send failed", "err", err, "text", ev.Text)
			return
		}
		slog.InfoContext(ctx, "chat.send: sent", "identity", ev.Identity, "text", ev.Text)
	default:
		slog.WarnContext(ctx, "chat.send: unknown identity dropped", "identity", ev.Identity)
	}
}
