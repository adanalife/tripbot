package chatbot

import (
	"context"

	"github.com/adanalife/tripbot/pkg/events"
)

// noopEvents satisfies Events for tests that don't assert on the durable
// event trail; every write succeeds and is discarded.
type noopEvents struct{}

func (noopEvents) Follow(_ context.Context, _ string) error                { return nil }
func (noopEvents) Subscribe(_ context.Context, _ string) error             { return nil }
func (noopEvents) Unsubscribe(_ context.Context, _ string) error           { return nil }
func (noopEvents) Raided(_ context.Context, _ events.Raid) error           { return nil }
func (noopEvents) Correction(_ context.Context, _ string, _ float64) error { return nil }
func (noopEvents) GuessSubmitted(_ context.Context, _ events.GuessSubmission) error {
	return nil
}
func (noopEvents) Timewarp(_ context.Context, _ events.Warp) error { return nil }
func (noopEvents) CommandRefused(_ context.Context, _ events.CommandRefusal) error {
	return nil
}
func (noopEvents) CommandRan(_ context.Context, _ events.CommandRun) error {
	return nil
}

// recordingEvents captures the event rows a run produced, so the emit sites
// can be asserted on without a database.
// recordedCorrection is one Correction call — the function takes loose args
// rather than a struct, so the recorder names them here.
type recordedCorrection struct {
	Username string
	Delta    float64
}

type recordingEvents struct {
	Follows   []string
	Subs      []string
	Unsubs    []string
	Guesses   []events.GuessSubmission
	Timewarps []events.Warp

	Corrections []recordedCorrection

	Refusals []events.CommandRefusal
	Raids    []events.Raid
	Runs     []events.CommandRun
}

func (r *recordingEvents) Correction(_ context.Context, username string, delta float64) error {
	r.Corrections = append(r.Corrections, recordedCorrection{username, delta})
	return nil
}

func (r *recordingEvents) GuessSubmitted(_ context.Context, g events.GuessSubmission) error {
	r.Guesses = append(r.Guesses, g)
	return nil
}

func (r *recordingEvents) Timewarp(_ context.Context, w events.Warp) error {
	r.Timewarps = append(r.Timewarps, w)
	return nil
}

func (r *recordingEvents) Follow(_ context.Context, username string) error {
	r.Follows = append(r.Follows, username)
	return nil
}

func (r *recordingEvents) Subscribe(_ context.Context, username string) error {
	r.Subs = append(r.Subs, username)
	return nil
}

func (r *recordingEvents) Unsubscribe(_ context.Context, username string) error {
	r.Unsubs = append(r.Unsubs, username)
	return nil
}

func (r *recordingEvents) CommandRefused(_ context.Context, ref events.CommandRefusal) error {
	r.Refusals = append(r.Refusals, ref)
	return nil
}

func (r *recordingEvents) Raided(_ context.Context, raid events.Raid) error {
	r.Raids = append(r.Raids, raid)
	return nil
}

func (r *recordingEvents) CommandRan(_ context.Context, run events.CommandRun) error {
	r.Runs = append(r.Runs, run)
	return nil
}
