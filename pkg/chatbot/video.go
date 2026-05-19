package chatbot

import (
	"context"

	"github.com/adanalife/tripbot/pkg/video"
)

// Video is the subset of the pkg/video surface that chatbot commands depend
// on. Tests inject a fake; production uses the package-backed realVideo
// adapter wired in defaultApp. Mirrors the Onscreens/VLC injection pattern.
type Video interface {
	// Current returns the video the system believes is currently playing,
	// without making any I/O calls. Reads pkg/video's package-level state.
	Current() video.Video
	// GetCurrentlyPlaying refreshes pkg/video's notion of what's currently
	// playing (an HTTP call to vlc-server in production), updates the
	// package-level state, and returns the resulting Video.
	GetCurrentlyPlaying(ctx context.Context) video.Video
	// FindRandomByState returns a random video filmed in the given US state.
	// Returns *terrors.NoFootageForStateError when no rows match.
	FindRandomByState(ctx context.Context, state string) (video.Video, error)
}

// realVideo delegates to pkg/video.
type realVideo struct{}

func (realVideo) Current() video.Video { return video.CurrentlyPlaying() }
func (realVideo) GetCurrentlyPlaying(ctx context.Context) video.Video {
	video.GetCurrentlyPlaying(ctx)
	return video.CurrentlyPlaying()
}
func (realVideo) FindRandomByState(ctx context.Context, state string) (video.Video, error) {
	return video.FindRandomByState(ctx, state)
}
