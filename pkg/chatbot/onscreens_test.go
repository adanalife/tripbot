package chatbot

import (
	"context"
	"fmt"
	"time"
)

// noopOnscreens satisfies Onscreens for tests that don't care about the
// overlay surface — it just swallows every call.
type noopOnscreens struct{}

func (noopOnscreens) ShowFlag(_ context.Context, _ time.Duration) error                      { return nil }
func (noopOnscreens) ShowLeaderboard(_ context.Context, _ string, _ [][]string) error        { return nil }
func (noopOnscreens) HideMiddleText(_ context.Context) error                                 { return nil }
func (noopOnscreens) ShowMiddleText(_ context.Context, _ string) error                       { return nil }
func (noopOnscreens) ShowTimewarp(_ context.Context) error                                   { return nil }

// recordingOnscreens captures every call made to it so tests can assert
// the chatbot invoked the expected overlay method with the expected args.
// All call records are appended in order to Calls.
type recordingOnscreens struct {
	Calls []string
}

func (r *recordingOnscreens) ShowFlag(_ context.Context, dur time.Duration) error {
	r.Calls = append(r.Calls, fmt.Sprintf("ShowFlag(%s)", dur))
	return nil
}
func (r *recordingOnscreens) ShowLeaderboard(_ context.Context, title string, lb [][]string) error {
	r.Calls = append(r.Calls, fmt.Sprintf("ShowLeaderboard(%q, %d rows)", title, len(lb)))
	return nil
}
func (r *recordingOnscreens) HideMiddleText(_ context.Context) error {
	r.Calls = append(r.Calls, "HideMiddleText()")
	return nil
}
func (r *recordingOnscreens) ShowMiddleText(_ context.Context, msg string) error {
	r.Calls = append(r.Calls, fmt.Sprintf("ShowMiddleText(%q)", msg))
	return nil
}
func (r *recordingOnscreens) ShowTimewarp(_ context.Context) error {
	r.Calls = append(r.Calls, "ShowTimewarp()")
	return nil
}
