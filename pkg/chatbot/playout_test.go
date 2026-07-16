package chatbot

import (
	"context"
	"fmt"
)

// noopPlayout satisfies Playout for tests that don't care about playback transitions
// — it just swallows every call.
type noopPlayout struct{}

func (noopPlayout) PlayRandom(_ context.Context) error                               { return nil }
func (noopPlayout) PlayFileInPlaylist(_ context.Context, _ string) error             { return nil }
func (noopPlayout) PlayFileAtTimestamp(_ context.Context, _ string, _ float64) error { return nil }
func (noopPlayout) Skip(_ context.Context, _ int) error                              { return nil }
func (noopPlayout) Back(_ context.Context, _ int) error                              { return nil }

// recordingPlayout captures every call made to it so tests can assert that
// the chatbot drove playback as expected. All call records are appended
// in order to Calls.
type recordingPlayout struct {
	Calls []string
}

func (r *recordingPlayout) PlayRandom(_ context.Context) error {
	r.Calls = append(r.Calls, "PlayRandom()")
	return nil
}
func (r *recordingPlayout) PlayFileInPlaylist(_ context.Context, filename string) error {
	r.Calls = append(r.Calls, fmt.Sprintf("PlayFileInPlaylist(%q)", filename))
	return nil
}
func (r *recordingPlayout) PlayFileAtTimestamp(_ context.Context, filename string, tsSec float64) error {
	r.Calls = append(r.Calls, fmt.Sprintf("PlayFileAtTimestamp(%q, %.1f)", filename, tsSec))
	return nil
}
func (r *recordingPlayout) Skip(_ context.Context, n int) error {
	r.Calls = append(r.Calls, fmt.Sprintf("Skip(%d)", n))
	return nil
}
func (r *recordingPlayout) Back(_ context.Context, n int) error {
	r.Calls = append(r.Calls, fmt.Sprintf("Back(%d)", n))
	return nil
}
