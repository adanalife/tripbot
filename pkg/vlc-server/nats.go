package vlcServer

import (
	"context"
	"encoding/json"
	"log/slog"

	c "github.com/adanalife/tripbot/pkg/config/vlc-server"
	"github.com/adanalife/tripbot/pkg/natsclient"
	ve "github.com/adanalife/tripbot/pkg/vlc-events"
	"github.com/nats-io/nats.go"
)

// StartNATSSubscribers attaches the server's NATS subscriptions to the
// package-singleton *nats.Conn (initialized by main via natsclient.Connect).
// No-op when the conn is nil (NATS_URL unset); HTTP remains the sole
// transport then.
//
// OBSERVE-ONLY (mirror phase): vlc commands are not idempotent — acting on
// both NATS and the mirrored HTTP call would double-execute (skip two videos,
// two random jumps). So unlike the onscreens subscribers, these handlers
// decode their payload and LOG what they would do; HTTP stays the sole actor.
// This burns in NATS delivery + wire format end-to-end without touching
// playback. The peel PR flips these to act (s.skip(n) etc.) and removes the
// HTTP command path in the same change — never a window where both act.
//
// Subscribers are registered explicitly (not via a vlc.> wildcard) so each
// gets its own subscribe log line and the dispatch stays readable.
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
		{ve.PlayRandomSubject(env), s.handlePlayRandom},
		{ve.PlayFileSubject(env), s.handlePlayFile},
		{ve.SkipSubject(env), s.handleSkip},
		{ve.BackSubject(env), s.handleBack},
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

// The handlers below are observe-only: they decode (validating the wire
// format and delivery) and log, but do not drive playback — see the
// StartNATSSubscribers doc. The "would" phrasing in the log is deliberate.

func (s *Server) handlePlayRandom(_ *nats.Msg) {
	slog.Info("nats: vlc play.random (observe-only, HTTP still acts)")
}

func (s *Server) handlePlayFile(m *nats.Msg) {
	var ev ve.PlayFile
	if err := json.Unmarshal(m.Data, &ev); err != nil {
		slog.Error("nats: decode vlc play.file", "err", err, "subject", m.Subject)
		return
	}
	slog.Info("nats: vlc play.file (observe-only, HTTP still acts)", "file", ev.File)
}

func (s *Server) handleSkip(m *nats.Msg) {
	var ev ve.Skip
	if err := json.Unmarshal(m.Data, &ev); err != nil {
		slog.Error("nats: decode vlc skip", "err", err, "subject", m.Subject)
		return
	}
	slog.Info("nats: vlc skip (observe-only, HTTP still acts)", "n", ev.N)
}

func (s *Server) handleBack(m *nats.Msg) {
	var ev ve.Back
	if err := json.Unmarshal(m.Data, &ev); err != nil {
		slog.Error("nats: decode vlc back", "err", err, "subject", m.Subject)
		return
	}
	slog.Info("nats: vlc back (observe-only, HTTP still acts)", "n", ev.N)
}
