package chatbot

import (
	"context"
	"sync"
	"time"

	"github.com/adanalife/tripbot/pkg/video"
)

// Video is the subset of the pkg/video surface that chatbot commands depend
// on. Tests inject a fake; production uses the realVideo adapter wired in
// defaultApp. Mirrors the Onscreens/VLC injection pattern.
type Video interface {
	// Current returns the video the system believes is currently playing,
	// without making any I/O calls.
	Current() video.Video
	// GetCurrentlyPlaying refreshes the Player's notion of what's currently
	// playing (an HTTP call to vlc-server in production) and returns it.
	GetCurrentlyPlaying(ctx context.Context) video.Video
	// CurrentProgress reports how long the current clip has been playing.
	CurrentProgress() time.Duration
	// FindRandomByState returns a random video filmed in the given US state.
	// Returns *terrors.NoFootageForStateError when no rows match.
	FindRandomByState(ctx context.Context, state string) (video.Video, error)
}

// videoPlayer is the *video.Player realVideo delegates to. cmd/tripbot installs
// the single process-wide instance via SetVideoPlayer once it's constructed, so
// commands read the same playback state the cron tick refreshes. nil until then
// (brief startup window) and in tests, which inject their own Video fake rather
// than realVideo — so the nil guards below only ever fire pre-install.
var (
	videoMu     sync.RWMutex
	videoPlayer *video.Player
)

// SetVideoPlayer installs the Player that realVideo delegates to. Called from
// cmd/tripbot once the Player is constructed. Mirrors SetScheduler.
func SetVideoPlayer(p *video.Player) {
	videoMu.Lock()
	videoPlayer = p
	videoMu.Unlock()
}

func currentPlayer() *video.Player {
	videoMu.RLock()
	defer videoMu.RUnlock()
	return videoPlayer
}

// realVideo delegates to the installed *video.Player (Current /
// GetCurrentlyPlaying) and to pkg/video's standalone DB helper
// (FindRandomByState, which is not Player state).
type realVideo struct{}

func (realVideo) Current() video.Video {
	p := currentPlayer()
	if p == nil {
		return video.Video{}
	}
	return p.Current()
}

func (realVideo) GetCurrentlyPlaying(ctx context.Context) video.Video {
	p := currentPlayer()
	if p == nil {
		return video.Video{}
	}
	p.GetCurrentlyPlaying(ctx)
	return p.Current()
}

func (realVideo) CurrentProgress() time.Duration {
	p := currentPlayer()
	if p == nil {
		return 0
	}
	return p.CurrentProgress()
}

func (realVideo) FindRandomByState(ctx context.Context, state string) (video.Video, error) {
	return video.FindRandomByState(ctx, state)
}
