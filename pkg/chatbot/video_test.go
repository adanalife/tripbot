package chatbot

import (
	"context"
	"fmt"
	"time"

	"github.com/adanalife/tripbot/pkg/video"
)

// recordingVideo captures every call made to it so tests can assert that the
// chatbot driver asked for the current video (or refreshed it). Vid is
// returned from Current/GetCurrentlyPlaying; RandomVid and RandomErr stage
// what FindRandomByState returns. Leave fields zero-valued unless a test
// cares. All call records are appended in order to Calls.
// Moment is handed back by PlayheadLocation, and only counts as a per-moment
// reading when PlayheadTracked is set — leave it false and the command under
// test takes the clip-level fix off Vid, which is what a clip with no trusted
// track does in production. Leave Moment's place fields empty to exercise the
// live-geocoder fallback, or set them to stand in for a geocoded row.
type recordingVideo struct {
	Calls           []string
	Vid             video.Video
	RandomVid       video.Video
	RandomErr       error
	DaytimeVid      video.Video
	DaytimeErr      error
	Moment          video.Moment
	PlayheadTracked bool
	// Bearing / Moving / HeadingTracked stage what PlayheadHeading reports:
	// a direction, a van sitting still, or a clip with no track to measure
	// along. HeadingTracked is separate from PlayheadTracked because a clip
	// can have a coordinate at the playhead and still have nothing a
	// headingLookback behind it.
	Bearing        float64
	Moving         bool
	HeadingTracked bool
	// RefreshedVid, when set, becomes Vid on the next GetCurrentlyPlaying —
	// staging a playhead jump that lands on a different clip, the way a real
	// refresh re-reads the player after playout shuffles.
	RefreshedVid *video.Video
}

func (r *recordingVideo) Current() video.Video {
	r.Calls = append(r.Calls, "Current()")
	return r.Vid
}
func (r *recordingVideo) GetCurrentlyPlaying(_ context.Context) video.Video {
	r.Calls = append(r.Calls, "GetCurrentlyPlaying()")
	if r.RefreshedVid != nil {
		r.Vid = *r.RefreshedVid
	}
	return r.Vid
}
func (r *recordingVideo) CurrentProgress() time.Duration {
	r.Calls = append(r.Calls, "CurrentProgress()")
	return 0
}
func (r *recordingVideo) PlayheadLocation(_ context.Context) (video.Video, video.Moment, bool) {
	r.Calls = append(r.Calls, "PlayheadLocation()")
	return r.Vid, r.Moment, r.PlayheadTracked
}
func (r *recordingVideo) PlayheadHeading(_ context.Context) (float64, bool, bool) {
	r.Calls = append(r.Calls, "PlayheadHeading()")
	return r.Bearing, r.Moving, r.HeadingTracked
}
func (r *recordingVideo) FindRandomByState(_ context.Context, state string) (video.Video, error) {
	r.Calls = append(r.Calls, fmt.Sprintf("FindRandomByState(%q)", state))
	return r.RandomVid, r.RandomErr
}
func (r *recordingVideo) FindNextDaytime(_ context.Context, after video.Video) (video.Video, error) {
	r.Calls = append(r.Calls, fmt.Sprintf("FindNextDaytime(%q)", after.Slug))
	return r.DaytimeVid, r.DaytimeErr
}
