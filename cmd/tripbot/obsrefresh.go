package main

import (
	"context"
	"encoding/json"
	"log/slog"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/natsclient"
	"github.com/adanalife/tripbot/pkg/obs"
	obsEvents "github.com/adanalife/tripbot/pkg/obs-events"
	"github.com/nats-io/nats.go"
)

// startOBSRefreshSubscriber wires the "hard-reload the OBS browser sources"
// command. The standalone console publishes obsEvents on
// tripbot.<env>.obs.refresh.<platform>; tripbot owns the OBS WebSocket, so it's
// the thing that actually reloads. It's the console-button counterpart to the
// !refreshoverlays chat command — the recovery for a crashed/frozen overlay a
// soft refresh can't revive.
//
// Per-platform: each tripbot instance subscribes to its own platform's leaf, so
// a Twitch-triggered refresh only touches obs-twitch. Unlike chat.send this runs
// for both platforms (each owns its own OBS), so it's wired before the YouTube
// early-return in run(). No-op when NATS is unconfigured. Must run after
// startNATS (conn).
func (t *Tripbot) startOBSRefreshSubscriber(ctx context.Context) {
	conn := natsclient.Conn()
	if conn == nil {
		slog.InfoContext(ctx, "obs.refresh subscriber skipped (NATS_URL unset)")
		return
	}
	subject := obsEvents.RefreshSubject(c.Conf.Environment, c.Conf.Platform)
	if _, err := conn.Subscribe(subject, func(m *nats.Msg) {
		var ev obsEvents.Refresh
		if err := json.Unmarshal(m.Data, &ev); err != nil {
			slog.ErrorContext(ctx, "obs.refresh: decode", "err", err, "subject", m.Subject)
			return
		}
		n, err := obs.RefreshBrowserSources(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "obs.refresh: reload failed", "err", err, "subject", m.Subject)
			return
		}
		slog.InfoContext(ctx, "obs.refresh: reloaded browser sources", "count", n)
	}); err != nil {
		slog.ErrorContext(ctx, "obs.refresh subscribe failed", "err", err, "subject", subject)
		return
	}
	slog.InfoContext(ctx, "nats subscribed", "subject", subject)
}
