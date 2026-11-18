package main

import (
	"context"
	"encoding/json"
	"log/slog"

	leaderboardEvents "github.com/adanalife/tripbot/pkg/leaderboard-events"
	"github.com/adanalife/tripbot/pkg/natsclient"
	"github.com/nats-io/nats.go"
)

// startLeaderboardShowSubscriber wires the "put this board on screen" command.
// The standalone console publishes leaderboardEvents.Show on
// tripbot.<env>.leaderboard.show.<platform> carrying a board name; tripbot owns
// the scoreboard tables, so it is the thing that turns that name into rows and
// publishes the onscreens command. It's the console-button counterpart to the
// !leaderboard family of chat commands, minus the chat line — an operator
// putting a board up is not a viewer asking for one.
//
// Per-platform: each tripbot instance subscribes to its own platform's leaf, so
// a board shown on Twitch leaves YouTube's overlay alone. Wired for both
// platforms (each drives its own overlays), so it goes before the YouTube
// early-return in run(). No-op when NATS is unconfigured. Must run after
// startNATS (conn).
func (t *Tripbot) startLeaderboardShowSubscriber(ctx context.Context) {
	conn := natsclient.Conn()
	if conn == nil {
		slog.InfoContext(ctx, "leaderboard.show subscriber skipped (NATS_URL unset)")
		return
	}
	subject := leaderboardEvents.ShowSubject(t.cfg.Environment, t.cfg.Platform)
	if _, err := conn.Subscribe(subject, func(m *nats.Msg) {
		var ev leaderboardEvents.Show
		if err := json.Unmarshal(m.Data, &ev); err != nil {
			slog.ErrorContext(ctx, "leaderboard.show: decode", "err", err, "subject", m.Subject)
			return
		}
		shown := t.app.ShowNamedLeaderboard(ctx, ev.Board)
		slog.InfoContext(ctx, "leaderboard.show handled", "board", ev.Board, "shown", shown)
	}); err != nil {
		slog.ErrorContext(ctx, "leaderboard.show subscribe failed", "err", err, "subject", subject)
		return
	}
	slog.InfoContext(ctx, "nats subscribed", "subject", subject)
}
