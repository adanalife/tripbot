package natsclient

import (
	"context"
	"log/slog"
)

// Publisher is the fire-and-forget publish seam consumers inject so they
// depend on an interface rather than reaching for the package singleton
// directly. The shape is identical to the publishers already embedded in
// pkg/eventbus and pkg/chatbot; new code takes a natsclient.Publisher so
// those can eventually collapse onto this one definition.
type Publisher interface {
	// Publish sends payload on subject. Errors are logged, never returned —
	// every publish is fire-and-forget by design. A no-op when NATS is
	// disabled (the singleton conn is nil).
	Publish(ctx context.Context, subject string, payload []byte)
}

// connPublisher delegates to the package singleton, read lazily on each
// call so a connection that lands after the caller is constructed (always —
// main runs after package vars) is still picked up.
type connPublisher struct{}

func (connPublisher) Publish(ctx context.Context, subject string, payload []byte) {
	c := Conn()
	if c == nil {
		return
	}
	if err := c.Publish(subject, payload); err != nil {
		slog.ErrorContext(ctx, "nats publish failed", "err", err, "subject", subject)
	}
}

// DefaultPublisher returns a Publisher backed by the package singleton.
// Callers that don't need their own seam (the video Player, background
// jobs) wire this in; tests inject a fake instead.
func DefaultPublisher() Publisher { return connPublisher{} }
