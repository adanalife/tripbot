package chatbot

import (
	"context"
)

// noopEvents satisfies Events for tests that don't assert on the durable
// event trail; every write succeeds and is discarded.
type noopEvents struct{}

func (noopEvents) Subscribe(_ context.Context, _ string) error             { return nil }
func (noopEvents) Unsubscribe(_ context.Context, _ string) error           { return nil }
func (noopEvents) Correction(_ context.Context, _ string, _ float64) error { return nil }
