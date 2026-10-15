package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/adanalife/tripbot/pkg/eventbus"
	"github.com/adanalife/tripbot/pkg/events"
	"github.com/adanalife/tripbot/pkg/natsclient"
	"github.com/nats-io/nats.go"
)

// startChatDeletedSubscriber strikes the chat log's copy of a line the platform
// removed. platform-gateway publishes eventbus.ChatDeleted on
// tripbot.<env>.chat.deleted for every platform; each instance applies only
// its own platform's removals, since chat_messages rows carry the platform
// that wrote them.
//
// A plain subscription, not a JetStream consumer: a removal published while
// this instance is down leaves the stored line unstruck. No-op when NATS is
// unconfigured. Must run after startNATS (conn).
func (t *Tripbot) startChatDeletedSubscriber(ctx context.Context) {
	conn := natsclient.Conn()
	if conn == nil {
		slog.InfoContext(ctx, "chat.deleted subscriber skipped (NATS_URL unset)")
		return
	}
	subject := eventbus.ChatDeletedSubject(t.cfg.Environment)
	if _, err := conn.Subscribe(subject, func(m *nats.Msg) {
		var ev eventbus.ChatDeleted
		if err := json.Unmarshal(m.Data, &ev); err != nil {
			slog.ErrorContext(ctx, "chat.deleted: decode", "err", err, "subject", m.Subject)
			return
		}
		if ev.Platform != t.cfg.Platform {
			return
		}
		at, err := time.Parse(time.RFC3339, ev.EmittedAt)
		if err != nil {
			at = time.Now()
		}
		n, err := events.StrikeChat(ctx, t.cfg, ev.UserID, ev.MessageID, at)
		if err != nil {
			slog.WarnContext(ctx, "chat.deleted: strike failed", "err", err, "user_id", ev.UserID, "message_id", ev.MessageID)
			return
		}
		slog.InfoContext(ctx, "chat.deleted: struck chat log lines", "rows", n, "user_id", ev.UserID, "message_id", ev.MessageID)
	}); err != nil {
		slog.ErrorContext(ctx, "chat.deleted subscribe failed", "err", err, "subject", subject)
		return
	}
	slog.InfoContext(ctx, "nats subscribed", "subject", subject)
}
