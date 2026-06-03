package onscreensServer

import (
	"context"
	"encoding/json"
	"log/slog"

	c "github.com/adanalife/tripbot/pkg/config/onscreens-server"
	"github.com/adanalife/tripbot/pkg/natsclient"
	oe "github.com/adanalife/tripbot/pkg/onscreens-events"
	"github.com/nats-io/nats.go"
)

// StartNATSSubscribers attaches the server's NATS subscriptions to the
// package-singleton *nats.Conn (initialized by main via natsclient.Connect).
// No-op when the conn is nil (NATS_URL unset); HTTP remains the sole
// transport then.
//
// Phase 2: the onscreens command surface moves onto NATS. Each handler maps
// 1-1 to the matching HTTP handler body, calling the same overlay method.
// The publisher mirrors every command alongside HTTP with HTTP still the
// source of truth, so acting here is presently redundant — but lets us burn
// in NATS delivery before peeling HTTP off. Subscribers are registered
// explicitly (not via an onscreens.> wildcard) so each gets its own
// subscribe log line and the dispatch stays readable.
func (s *Server) StartNATSSubscribers(ctx context.Context) {
	conn := natsclient.Conn()
	if conn == nil {
		slog.InfoContext(ctx, "nats subscriber skipped (NATS_URL unset)")
		return
	}
	env := c.Conf.Environment
	subs := []struct {
		subject string
		handler nats.MsgHandler
	}{
		{oe.MiddleShowSubject(env), s.handleMiddleShow},
		{oe.MiddleHideSubject(env), s.handleMiddleHide},
		{oe.LeaderboardShowSubject(env), s.handleLeaderboardShow},
		{oe.LeaderboardHideSubject(env), s.handleLeaderboardHide},
		{oe.TimewarpShowSubject(env), s.handleTimewarpShow},
		{oe.TimewarpHideSubject(env), s.handleTimewarpHide},
		{oe.GPSShowSubject(env), s.handleGPSShow},
		{oe.GPSHideSubject(env), s.handleGPSHide},
		{oe.FlagShowSubject(env), s.handleFlagShow},
		{oe.FlagHideSubject(env), s.handleFlagHide},
	}
	for _, sb := range subs {
		// Best-effort: one bad subject shouldn't stop the rest from binding.
		if _, err := conn.Subscribe(sb.subject, sb.handler); err != nil {
			slog.ErrorContext(ctx, "nats subscribe failed", "err", err, "subject", sb.subject)
			continue
		}
		slog.InfoContext(ctx, "nats subscribed", "subject", sb.subject)
	}
}

// handleMiddleShow shows the middle text. Strict: a missing msg or malformed
// body is dropped — a show with no content is a publisher bug, not an intent.
func (s *Server) handleMiddleShow(m *nats.Msg) {
	var ev oe.MiddleShow
	if err := json.Unmarshal(m.Data, &ev); err != nil {
		slog.Error("nats: decode middle.show", "err", err, "subject", m.Subject)
		return
	}
	if ev.Msg == "" {
		slog.Warn("nats: middle.show missing msg", "subject", m.Subject)
		return
	}
	s.MiddleText.Show(ev.Msg)
}

// handleLeaderboardShow renders the {title, rows} payload server-side and
// shows it for the standard duration — the same path the HTTP handler takes.
func (s *Server) handleLeaderboardShow(m *nats.Msg) {
	var ev oe.LeaderboardShow
	if err := json.Unmarshal(m.Data, &ev); err != nil {
		slog.Error("nats: decode leaderboard.show", "err", err, "subject", m.Subject)
		return
	}
	s.Leaderboard.ShowFor(renderLeaderboard(ev.Title, ev.Rows), leaderboardDuration)
}

// handleFlagShow stores the state abbrev on the flag onscreen and shows it
// for the standard duration; the flag asset handler resolves Content to the
// embedded per-state image. Strict: a missing state is a publisher bug.
func (s *Server) handleFlagShow(m *nats.Msg) {
	var ev oe.FlagShow
	if err := json.Unmarshal(m.Data, &ev); err != nil {
		slog.Error("nats: decode flag.show", "err", err, "subject", m.Subject)
		return
	}
	if ev.State == "" {
		slog.Warn("nats: flag.show missing state", "subject", m.Subject)
		return
	}
	s.Flag.ShowFor(ev.State, flagDuration)
}

// The hide + empty-payload show handlers below take no data beyond the
// envelope, so the body isn't inspected: the subject is the whole intent.
// Hides are lenient by construction (nothing to reject).
func (s *Server) handleMiddleHide(_ *nats.Msg)      { s.MiddleText.Hide() }
func (s *Server) handleLeaderboardHide(_ *nats.Msg) { s.Leaderboard.Hide() }
func (s *Server) handleTimewarpShow(_ *nats.Msg)    { s.Timewarp.ShowFor("Timewarp!", timewarpDuration) }
func (s *Server) handleTimewarpHide(_ *nats.Msg)    { s.Timewarp.Hide() }
func (s *Server) handleGPSShow(_ *nats.Msg)         { s.GPS.Show("") }
func (s *Server) handleGPSHide(_ *nats.Msg)         { s.GPS.Hide() }
func (s *Server) handleFlagHide(_ *nats.Msg)        { s.Flag.Hide() }
