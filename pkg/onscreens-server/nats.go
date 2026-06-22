package onscreensServer

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/onscreens-server"
	"github.com/adanalife/tripbot/pkg/natsclient"
	oe "github.com/adanalife/tripbot/pkg/onscreens-events"
	"github.com/nats-io/nats.go"
)

// StartNATSSubscribers attaches the server's NATS subscriptions to the
// package-singleton *nats.Conn (initialized by main via natsclient.Connect).
// No-op when the conn is nil (NATS_URL unset) — with the HTTP command path
// peeled off, the overlays simply receive no commands in that case.
//
// NATS is the sole transport for the onscreens command surface: each handler
// drives the overlay the matching command targets. (The HTTP command path was
// the mirror this burned in against, since peeled off.) Subscribers are
// registered explicitly (not via an onscreens.> wildcard) so each gets its own
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
		{oe.FlagHideSubject(env), s.handleFlagHide},
		{oe.LocationUpdateSubject(env), s.handleLocationUpdate},
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
	// Persist the new state so the overlay survives a server restart.
	publishMiddleState(context.Background(), c.Conf.Environment, ev.Msg, true)
}

// handleMiddleHide hides the middle text. Hide retains the overlay's Content,
// so the persisted state keeps the text (showing=false) — a restart restores
// it hidden, matching the live state rather than blanking it.
func (s *Server) handleMiddleHide(_ *nats.Msg) {
	s.MiddleText.Hide()
	publishMiddleState(context.Background(), c.Conf.Environment, s.MiddleText.Content, false)
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

// The hide + empty-payload show handlers below take no data beyond the
// envelope, so the body isn't inspected: the subject is the whole intent.
// Hides are lenient by construction (nothing to reject).
func (s *Server) handleLeaderboardHide(_ *nats.Msg) { s.Leaderboard.Hide() }
func (s *Server) handleTimewarpHide(_ *nats.Msg)    { s.Timewarp.Hide() }
func (s *Server) handleGPSShow(_ *nats.Msg)         { s.GPS.Show("") }
func (s *Server) handleGPSHide(_ *nats.Msg)         { s.GPS.Hide() }
func (s *Server) handleFlagHide(_ *nats.Msg)        { s.Flag.Hide() }

// handleLocationUpdate caches the currently-playing clip's location + date so
// the bot-less rotators can surface it. Lenient: a malformed body is dropped;
// empty fields are allowed (the rotator skips whichever line is empty).
func (s *Server) handleLocationUpdate(m *nats.Msg) {
	var ev oe.LocationData
	if err := json.Unmarshal(m.Data, &ev); err != nil {
		slog.Error("nats: decode location.update", "err", err, "subject", m.Subject)
		return
	}
	liveLocation.set(ev.Location, ev.Date, time.Now())
}

// handleTimewarpShow triggers the full-screen warp. The overlay's Content
// carries the triggering chatter's username (lenient: a malformed body or a
// missing username just yields no credit line — the warp still plays). The
// browser source reads Content to render the "@username" credit under the
// TIMEWARP wordmark.
func (s *Server) handleTimewarpShow(m *nats.Msg) {
	var ev oe.TimewarpShow
	if err := json.Unmarshal(m.Data, &ev); err != nil {
		slog.Error("nats: decode timewarp.show", "err", err, "subject", m.Subject)
		return
	}
	s.Timewarp.ShowFor(ev.Username, timewarpDuration)
}
