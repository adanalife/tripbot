package chatbot

import (
	"context"
	"time"

	"github.com/adanalife/tripbot/pkg/video"
)

// Video is the subset of the pkg/video surface that chatbot commands depend
// on. Tests inject a fake; production uses the realVideo adapter, which
// cmd/tripbot builds around the process-wide *video.Player via NewVideoAdapter.
// Mirrors the Onscreens/Playout injection pattern.
type Video interface {
	// Current returns the video the system believes is currently playing,
	// without making any I/O calls.
	Current() video.Video
	// GetCurrentlyPlaying refreshes the Player's notion of what's currently
	// playing (an HTTP call to playout in production) and returns it.
	GetCurrentlyPlaying(ctx context.Context) video.Video
	// CurrentProgress reports how long the current clip has been playing.
	CurrentProgress() time.Duration
	// FindRandomByState returns a random video filmed in the given US state.
	// Returns *terrors.NoFootageForStateError when no rows match.
	FindRandomByState(ctx context.Context, state string) (video.Video, error)
}

// realVideo delegates to its *video.Player (Current / GetCurrentlyPlaying /
// CurrentProgress) and to pkg/video's standalone DB helper (FindRandomByState,
// which is not Player state). cmd/tripbot installs the process-wide Player via
// NewVideoAdapter so commands read the same playback state the cron tick
// refreshes. player is nil in New()'s default adapter until cmd assigns
// App.Video, so the nil guards below cover that brief startup window. Tests
// inject their own Video fake rather than realVideo, so the guards only ever
// fire pre-install.
type realVideo struct{ player *video.Player }

// NewVideoAdapter builds the production Video adapter around p. cmd/tripbot
// assigns the result onto App.Video once the Player is constructed.
func NewVideoAdapter(p *video.Player) Video { return realVideo{player: p} }

func (r realVideo) Current() video.Video {
	if r.player == nil {
		return video.Video{}
	}
	return r.player.Current()
}

func (r realVideo) GetCurrentlyPlaying(ctx context.Context) video.Video {
	if r.player == nil {
		return video.Video{}
	}
	r.player.GetCurrentlyPlaying(ctx)
	return r.player.Current()
}

func (r realVideo) CurrentProgress() time.Duration {
	if r.player == nil {
		return 0
	}
	return r.player.CurrentProgress()
}

func (r realVideo) FindRandomByState(ctx context.Context, state string) (video.Video, error) {
	return video.FindRandomByState(ctx, state)
}
