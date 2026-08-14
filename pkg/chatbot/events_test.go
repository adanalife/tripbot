package chatbot

import (
	"context"

	"github.com/adanalife/tripbot/pkg/events"
)

// noopEvents satisfies Events for tests that don't assert on the durable
// event trail; every write succeeds and is discarded.
type noopEvents struct{}

func (noopEvents) Subscribe(_ context.Context, _ string) error             { return nil }
func (noopEvents) Unsubscribe(_ context.Context, _ string) error           { return nil }
func (noopEvents) Correction(_ context.Context, _ string, _ float64) error { return nil }
func (noopEvents) CommandRefused(_ context.Context, _ events.CommandRefusal) error {
	return nil
}
func (noopEvents) CommandRan(_ context.Context, _ events.CommandRun) error {
	return nil
}

// recordingEvents captures the event rows a run produced, so the emit sites
// can be asserted on without a database.
type recordingEvents struct {
	noopEvents
	Refusals []events.CommandRefusal
	Runs     []events.CommandRun
}

func (r *recordingEvents) CommandRefused(_ context.Context, ref events.CommandRefusal) error {
	r.Refusals = append(r.Refusals, ref)
	return nil
}

func (r *recordingEvents) CommandRan(_ context.Context, run events.CommandRun) error {
	r.Runs = append(r.Runs, run)
	return nil
}
