package onscreensServer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	c "github.com/adanalife/tripbot/pkg/config/onscreens-server"
	"github.com/adanalife/tripbot/pkg/natsclient"
	"github.com/nats-io/nats.go"
)

// middleTextEvent mirrors the chatbot-side wire format for
// tripbot.<env>.onscreens.middle.show. Mirror, not import, to keep the
// chatbot/onscreens-server package boundary clean — the wire format
// will live in its own pkg/events module once a second event lands.
type middleTextEvent struct {
	Msg       string `json:"msg"`
	EmittedAt string `json:"emitted_at"`
}

// StartNATSSubscribers attaches the server's NATS subscriptions to the
// package-singleton *nats.Conn (initialized by main via
// natsclient.Connect). No-op when the conn is nil (NATS_URL unset).
// Returns the subscription so callers can Unsubscribe on shutdown if
// they want — phase 1 lets the process exit do that work.
//
// Phase 1: a single subject — tripbot.<env>.onscreens.middle.show. Phase
// 2 peels the rest of the onscreens-client surface onto NATS.
func (s *Server) StartNATSSubscribers(ctx context.Context) {
	conn := natsclient.Conn()
	if conn == nil {
		slog.InfoContext(ctx, "nats subscriber skipped (NATS_URL unset)")
		return
	}
	subject := fmt.Sprintf("tripbot.%s.onscreens.middle.show", c.Conf.Environment)
	sub, err := conn.Subscribe(subject, s.handleMiddleTextShow)
	if err != nil {
		slog.ErrorContext(ctx, "nats subscribe failed", "err", err, "subject", subject)
		return
	}
	slog.InfoContext(ctx, "nats subscribed", "subject", subject, "queue", sub.Queue)
}

func (s *Server) handleMiddleTextShow(m *nats.Msg) {
	var ev middleTextEvent
	if err := json.Unmarshal(m.Data, &ev); err != nil {
		slog.Error("nats: decode middle-text event", "err", err, "subject", m.Subject)
		return
	}
	if ev.Msg == "" {
		slog.Warn("nats: middle-text event missing msg field", "subject", m.Subject)
		return
	}
	s.MiddleText.Show(ev.Msg)
}
