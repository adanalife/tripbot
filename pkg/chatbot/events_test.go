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
func (noopEvents) GuessSubmitted(_ context.Context, _ events.GuessSubmission) error {
	return nil
}
func (noopEvents) Timewarp(_ context.Context, _ events.Warp) error { return nil }
func (noopEvents) CommandRefused(_ context.Context, _ events.CommandRefusal) error {
	return nil
}

// recordingEvents captures the refusals a run produced, so the emit sites can be
// asserted on without a database.
type recordingEvents struct {
	noopEvents
	Guesses   []events.GuessSubmission
	Timewarps []events.Warp

	Refusals []events.CommandRefusal
}

func (r *recordingEvents) GuessSubmitted(_ context.Context, g events.GuessSubmission) error {
	r.Guesses = append(r.Guesses, g)
	return nil
}

func (r *recordingEvents) Timewarp(_ context.Context, w events.Warp) error {
	r.Timewarps = append(r.Timewarps, w)
	return nil
}

func (r *recordingEvents) CommandRefused(_ context.Context, ref events.CommandRefusal) error {
	r.Refusals = append(r.Refusals, ref)
	return nil
}
