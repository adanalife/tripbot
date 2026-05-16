package vlcServer

import (
	"context"
	"log/slog"
	"time"

	"github.com/adanalife/tripbot/pkg/instrumentation"
)

// PollStats polls libvlc for playback statistics every interval and pushes
// them through pkg/instrumentation. Intended to run as a long-lived
// goroutine started after InitPlayer.
//
// libvlc resets these counters when the current Media changes; we detect
// that by watching for a negative delta in DisplayedPictures and skip the
// FPS sample on that tick to avoid a phantom dip.
func PollStats(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var (
		prevDisplayed float64
		prevTime      time.Time
		havePrev      bool
	)

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if player == nil {
				continue
			}
			media, err := player.Media()
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

// StartStatsPoller is a tiny launcher wrapper so cmd/vlc-server can `go
// vlcServer.StartStatsPoller(...)` without juggling its own goroutine.
func StartStatsPoller(ctx context.Context, interval time.Duration) {
	slog.InfoContext(ctx, "starting libvlc stats poller", "interval", interval)
	go PollStats(ctx, interval)
}
