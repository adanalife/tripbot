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
// No-op when the conn is nil (NATS_URL unset) — with the HTTP command path
// peeled off, the server simply receives no commands in that case.
//
// NATS is the sole transport for the vlc command surface: each handler drives
// the same playback method the old HTTP handler called. (The observe-only
// mirror this burned in against, and the HTTP command path, have since been
// peeled off.) The publish-only client no longer calls HTTP, so there's no
// double-execution despite vlc commands not being idempotent.
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

// Each handler maps 1-1 to the playback method the old HTTP handler called.
// play.file is strict (an empty filename is a publisher bug, dropped); skip /
// back normalize a non-positive count to 1, matching the old HTTP handler's
// default when no n was supplied.

func (s *Server) handlePlayRandom(_ *nats.Msg) {
	if err := s.PlayRandom(); err != nil {
		slog.Error("nats: vlc play.random failed", "err", err)
	}
}

func (s *Server) handlePlayFile(m *nats.Msg) {
	var ev ve.PlayFile
	if err := json.Unmarshal(m.Data, &ev); err != nil {
		slog.Error("nats: decode vlc play.file", "err", err, "subject", m.Subject)
		return
	}
	if ev.File == "" {
		slog.Warn("nats: vlc play.file missing file", "subject", m.Subject)
		return
	}
	if err := s.PlayVideoFile(ev.File); err != nil {
		slog.Error("nats: vlc play.file failed", "err", err, "file", ev.File)
	}
}

func (s *Server) handleSkip(m *nats.Msg) {
	var ev ve.Skip
	if err := json.Unmarshal(m.Data, &ev); err != nil {
		slog.Error("nats: decode vlc skip", "err", err, "subject", m.Subject)
		return
	}
	n := ev.N
	if n <= 0 {
		n = 1
	}
	s.skip(n)
}

func (s *Server) handleBack(m *nats.Msg) {
	var ev ve.Back
	if err := json.Unmarshal(m.Data, &ev); err != nil {
		slog.Error("nats: decode vlc back", "err", err, "subject", m.Subject)
		return
	}
	n := ev.N
	if n <= 0 {
		n = 1
	}
	s.back(n)
}
