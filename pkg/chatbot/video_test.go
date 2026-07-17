package chatbot

import (
	"context"
	"fmt"
	"time"

	"github.com/adanalife/tripbot/pkg/video"
)

// noopVideo satisfies Video for tests that don't care about the currently-
// playing video — it just returns the zero value for every call.
type noopVideo struct{}

func (noopVideo) Current() video.Video                              { return video.Video{} }
func (noopVideo) GetCurrentlyPlaying(_ context.Context) video.Video { return video.Video{} }
func (noopVideo) CurrentProgress() time.Duration                    { return 0 }
func (noopVideo) FindRandomByState(_ context.Context, _ string) (video.Video, error) {
	return video.Video{}, nil
}
func (noopVideo) FindNextDaytime(_ context.Context, _ video.Video) (video.Video, error) {
	return video.Video{}, nil
}

// recordingVideo captures every call made to it so tests can assert that the
// chatbot driver asked for the current video (or refreshed it). Vid is
// returned from Current/GetCurrentlyPlaying; RandomVid and RandomErr stage
// what FindRandomByState returns. Leave fields zero-valued unless a test
// cares. All call records are appended in order to Calls.
type recordingVideo struct {
	Calls      []string
	Vid        video.Video
	RandomVid  video.Video
	RandomErr  error
	DaytimeVid video.Video
	DaytimeErr error
}

func (r *recordingVideo) Current() video.Video {
	r.Calls = append(r.Calls, "Current()")
	return r.Vid
}
func (r *recordingVideo) GetCurrentlyPlaying(_ context.Context) video.Video {
	r.Calls = append(r.Calls, "GetCurrentlyPlaying()")
	return r.Vid
}
func (r *recordingVideo) CurrentProgress() time.Duration {
	r.Calls = append(r.Calls, "CurrentProgress()")
	return 0
}
func (r *recordingVideo) FindRandomByState(_ context.Context, state string) (video.Video, error) {
	r.Calls = append(r.Calls, fmt.Sprintf("FindRandomByState(%q)", state))
	return r.RandomVid, r.RandomErr
}
func (r *recordingVideo) FindNextDaytime(_ context.Context, after video.Video) (video.Video, error) {
	r.Calls = append(r.Calls, fmt.Sprintf("FindNextDaytime(%q)", after.Slug))
	return r.DaytimeVid, r.DaytimeErr
}
