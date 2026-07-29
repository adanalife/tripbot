package chatbot

import (
	"context"
	"fmt"
)

// noopEvents satisfies Events for tests that don't assert on the durable
// event trail; every write succeeds and is discarded.
type noopEvents struct{}

func (noopEvents) Subscribe(_ context.Context, _ string) error             { return nil }
func (noopEvents) Unsubscribe(_ context.Context, _ string) error           { return nil }
func (noopEvents) Correction(_ context.Context, _ string, _ float64) error { return nil }

// recordingEvents captures the events written to it, so a test can assert the
// durable trail was recorded without a database. Err, when set, fails every
// write — the path where the command has already acted and only the record is
// lost.
type recordingEvents struct {
	Calls []string
	Err   error
}

func (r *recordingEvents) Subscribe(_ context.Context, username string) error {
	r.Calls = append(r.Calls, fmt.Sprintf("Subscribe(%q)", username))
	return r.Err
}

func (r *recordingEvents) Unsubscribe(_ context.Context, username string) error {
	r.Calls = append(r.Calls, fmt.Sprintf("Unsubscribe(%q)", username))
	return r.Err
}

func (r *recordingEvents) Correction(_ context.Context, username string, delta float64) error {
	r.Calls = append(r.Calls, fmt.Sprintf("Correction(%q, %v)", username, delta))
	return r.Err
}
