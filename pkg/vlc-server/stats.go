package vlcServer

import (
	"context"
	"log/slog"
	"time"

	"github.com/adanalife/tripbot/pkg/instrumentation"
	libvlc "github.com/adrg/libvlc-go/v3"
)

// pollStats polls libvlc for playback statistics every interval and pushes
// them through pkg/instrumentation. Intended to run as a long-lived
// goroutine started after New.
//
// libvlc resets these counters when the current Media changes; we detect
// that by watching for a negative delta in DisplayedPictures and skip the
// FPS sample on that tick to avoid a phantom dip.
func (s *Server) pollStats(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var (
		prevDisplayed float64
		prevTime      time.Time
		havePrev      bool

		prevState     libvlc.MediaState
		havePrevState bool
	)

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if s.Player == nil {
				continue
			}

			// Count player-state transitions (playing↔paused↔buffering↔…).
			// Done before the media/stats fetch below so states with no
			// media (stopped/ended/error) are still observed. Inc only on a
			// genuine change, so the counter tracks transitions not ticks.
			if state, err := s.Player.MediaState(); err == nil {
				if havePrevState && state != prevState {
					instrumentation.VLCStateTransitions.Inc(vlcStateLabel(state))
				}
				prevState = state
				havePrevState = true
			}

			media, err := s.Player.Media()
			if err != nil || media == nil {
				havePrev = false
				continue
			}
			stats, err := media.Stats()
			if err != nil || stats == nil {
				havePrev = false
				continue
			}

			displayed := float64(stats.DisplayedPictures)
			var fps float64
			if havePrev {
				dt := now.Sub(prevTime).Seconds()
				delta := displayed - prevDisplayed
				if dt > 0 && delta >= 0 {
					fps = delta / dt
				}
				// delta < 0 means libvlc rolled over to a new Media —
				// skip this sample's FPS and let the next tick reseed.
			}
			prevDisplayed = displayed
			prevTime = now
			havePrev = true

			instrumentation.VLCPlayerStats.Update(instrumentation.VLCPlayerStatsSnapshot{
				InputBitRate:       stats.InputBitRate,
				DemuxBitRate:       stats.DemuxBitRate,
				DisplayedFPS:       fps,
				DecodedVideo:       float64(stats.DecodedVideo),
				DisplayedPictures:  displayed,
				LostPictures:       float64(stats.LostPictures),
				DemuxCorrupted:     float64(stats.DemuxCorrupted),
				DemuxDiscontinuity: float64(stats.DemuxDiscontinuity),
			})
		}
	}
}

// vlcStateLabel maps a libvlc MediaState to a stable, low-cardinality metric
// label. libvlc.MediaState has no String(), so map it explicitly.
func vlcStateLabel(state libvlc.MediaState) string {
	switch state {
	case libvlc.MediaNothingSpecial:
		return "nothing_special"
	case libvlc.MediaOpening:
		return "opening"
	case libvlc.MediaBuffering:
		return "buffering"
	case libvlc.MediaPlaying:
		return "playing"
	case libvlc.MediaPaused:
		return "paused"
	case libvlc.MediaStopped:
		return "stopped"
	case libvlc.MediaEnded:
		return "ended"
	case libvlc.MediaError:
		return "error"
	default:
		return "unknown"
	}
}

// StartStatsPoller is a tiny launcher wrapper so cmd/vlc-server can
// `srv.StartStatsPoller(...)` without juggling its own goroutine.
func (s *Server) StartStatsPoller(ctx context.Context, interval time.Duration) {
	slog.InfoContext(ctx, "starting libvlc stats poller", "interval", interval)
	go s.pollStats(ctx, interval)
}
