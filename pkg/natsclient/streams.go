package natsclient

import (
	"context"
	"fmt"
	"log/slog"
	"time"

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

// Stream declares are retried this many times, the wait doubling from
// declareRetryBase: about 30s in all, which covers JetStream still recovering
// its store right after a NATS restart without pinning a callback goroutine
// for long.
const (
	declareAttempts  = 6
	declareRetryBase = time.Second
)

// RetryDeclare runs a stream declare until it succeeds, giving up after a
// bounded backoff or when ctx ends, and returns the last error. Meant for the
// on-connect callbacks, where a declare fails while JetStream is still coming
// up and the next chance would otherwise be the next reconnect.
func RetryDeclare(ctx context.Context, declare func(context.Context) error) error {
	return retry(ctx, declareAttempts, declareRetryBase, declare)
}

func retry(ctx context.Context, attempts int, wait time.Duration, fn func(context.Context) error) error {
	var err error
	for i := 0; i < attempts; i++ {
		if err = fn(ctx); err == nil {
			return nil
		}
		if i == attempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(wait):
		}
		wait *= 2
	}
	return err
}
