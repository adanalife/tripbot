package chatbot

import (
	"context"
	"log/slog"

	"github.com/adanalife/tripbot/pkg/natsclient"
)

// NATS is the publish surface chatbot uses for fire-and-forget pubsub
// events. Tests inject a fake (recordingNATS / noopNATS); production
// uses realNATS which delegates to the pkg/natsclient singleton.
//
// Phase 1: a single call site (realOnscreens.ShowMiddleText) publishes
// alongside the HTTP path. Phase 2 peels more onscreens-client surface
// onto NATS; phase 3 adds JetStream for events that need replay.
type NATS interface {
	// Publish sends payload on subject. Errors are logged but never
	// returned — every chatbot publish is fire-and-forget by design.
	// When NATS is disabled (NATS_URL empty), the call no-ops silently.
	Publish(ctx context.Context, subject string, payload []byte)
}

// realNATS delegates to whatever *nats.Conn pkg/natsclient currently
// holds. Lazy read on each Publish so a connection that lands after
// chatbot init (i.e. always — main runs after package vars) is picked up.
type realNATS struct{}

func (realNATS) Publish(ctx context.Context, subject string, payload []byte) {
	c := natsclient.Conn()
	if c == nil {
		return
	}
	if err := c.Publish(subject, payload); err != nil {
		slog.ErrorContext(ctx, "nats publish failed", "err", err, "subject", subject)
	}
}
