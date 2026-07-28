package natsclient

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"
)

// EnsureLastValueStream idempotently declares a last-value-cache stream: file
// storage, one retained message per subject, so the newest publish on each
// subject is always readable and nothing else accumulates. It's the shape used
// for "restore this on restart" state (the middle-text overlay, the rotator
// copy, the playout lastplayed cache).
//
// Both the publisher and the reader should call this before their first
// publish/read. A core publish to a subject no stream covers is silently
// uncaptured, so whichever side happens to start first still gets the stream
// declared; CreateOrUpdateStream makes the second call a no-op.
//
// No-op when js is nil (NATS off / JetStream unavailable) — callers then degrade
// to whatever their non-persistent fallback is.
func EnsureLastValueStream(ctx context.Context, js jetstream.JetStream, name, description string, subjects []string) error {
	if js == nil {
		return nil
	}
	cfg := jetstream.StreamConfig{
		Name:        name,
		Description: description,
		Subjects:    subjects,
		Storage:     jetstream.FileStorage,
		Retention:   jetstream.LimitsPolicy,
		Discard:     jetstream.DiscardOld,
		// One retained message per subject: exactly the latest state, nothing
		// to prune.
		MaxMsgsPerSubject: 1,
	}
	if _, err := js.CreateOrUpdateStream(ctx, cfg); err != nil {
		return fmt.Errorf("ensure stream %s: %w", name, err)
	}
	slog.InfoContext(ctx, "jetstream stream ensured", "stream", name)
	return nil
}
