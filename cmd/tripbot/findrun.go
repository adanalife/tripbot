package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	findEvents "github.com/adanalife/tripbot/pkg/find-events"
	"github.com/adanalife/tripbot/pkg/natsclient"
	"github.com/nats-io/nats.go"
)

// findRunTimeout bounds one operator find: the embed request alone may take
// findEmbedTimeout (10s), then the warp cover and the jump.
const findRunTimeout = 30 * time.Second

// startFindRunSubscriber answers the operator find command on
// tripbot.<env>.find.run.<platform>: the chat-free !find, replying with a
// findEvents.Result. Per-platform, like obs.refresh — each instance drives its
// own playout. No-op when NATS is unconfigured.
func (t *Tripbot) startFindRunSubscriber(ctx context.Context) {
	conn := natsclient.Conn()
	if conn == nil {
		slog.InfoContext(ctx, "find.run subscriber skipped (NATS_URL unset)")
		return
	}
	subject := findEvents.RunSubject(t.cfg.Environment, t.cfg.Platform)
	if _, err := conn.Subscribe(subject, func(m *nats.Msg) {
		var req findEvents.Run
		if err := json.Unmarshal(m.Data, &req); err != nil {
			slog.ErrorContext(ctx, "find.run: decode", "err", err, "subject", m.Subject)
			return
		}
		runCtx, cancel := context.WithTimeout(ctx, findRunTimeout)
		defer cancel()
		ok, detail := t.app.Find(runCtx, req.Query)
		slog.InfoContext(ctx, "find.run", "query", req.Query, "ok", ok, "detail", detail)
		reply, err := json.Marshal(findEvents.Result{OK: ok, Detail: detail})
		if err != nil {
			slog.ErrorContext(ctx, "find.run: encode", "err", err)
			return
		}
		if err := m.Respond(reply); err != nil {
			slog.ErrorContext(ctx, "find.run: respond", "err", err, "subject", m.Subject)
		}
	}); err != nil {
		slog.ErrorContext(ctx, "find.run subscribe failed", "err", err, "subject", subject)
		return
	}
	slog.InfoContext(ctx, "nats subscribed", "subject", subject)
}
