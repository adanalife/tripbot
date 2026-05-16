package chatbot

import (
	"github.com/adanalife/tripbot/pkg/video"
)

// noopVideo satisfies Video for tests that don't care about the currently-
// playing video — it just returns the zero value for every call.
type noopVideo struct{}

func (noopVideo) Current() video.Video             { return video.Video{} }
func (noopVideo) GetCurrentlyPlaying() video.Video { return video.Video{} }

// recordingVideo captures every call made to it so tests can assert that the
// chatbot driver asked for the current video (or refreshed it). Vid is
// returned from both methods; leave it zero-valued unless a test cares.
// All call records are appended in order to Calls.
type recordingVideo struct {
	Calls []string
	Vid   video.Video
}

func (r *recordingVideo) Current() video.Video {
	r.Calls = append(r.Calls, "Current()")
	return r.Vid
}
func (r *recordingVideo) GetCurrentlyPlaying() video.Video {
	r.Calls = append(r.Calls, "GetCurrentlyPlaying()")
	return r.Vid
}
