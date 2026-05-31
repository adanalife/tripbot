package vlcServer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/vlc-server"
)

const (
	// nextFrameFileName is the single-slot cover-frame cache filename.
	// Lives in RunDir alongside the pidfile so it shares the same
	// writable mount and gets cleaned up on pod restart.
	nextFrameFileName = "next-frame.jpg"

	// ffmpegExtractTimeout caps each extraction so a wedged ffmpeg can't
	// stick the refresher goroutine.
	ffmpegExtractTimeout = 10 * time.Second
)

// NextFrameCachePath returns the absolute path to the single-slot
// cover-frame cache.
func NextFrameCachePath() string {
	return filepath.Join(c.Conf.RunDir, nextFrameFileName)
}

// ExtractFirstFrame writes the first frame of videoPath to outPath as a
// JPEG via ffmpeg. The write is atomic: ffmpeg writes to a sibling .tmp
// file which is renamed on success, so an HTTP reader never sees a
// partial JPEG.
//
// -ss 0 before -i for fast seek; -frames:v 1 to grab one frame; -q:v 3
// is a sane JPEG quality (2-5 range); -nostdin so a stalled ffmpeg can't
// read from vlc-server's stdin; -y to overwrite the .tmp from any prior
// interrupted extraction. -f image2 -update 1 selects the muxer
// explicitly and says "single image, not a sequence" so ffmpeg doesn't
// try to infer the muxer from the .tmp file extension or treat the path
// as a numbered-sequence pattern.
func ExtractFirstFrame(ctx context.Context, videoPath, outPath string) error {
	tmpPath := outPath + ".tmp"
	defer os.Remove(tmpPath) // best-effort; harmless after a successful rename

	cctx, cancel := context.WithTimeout(ctx, ffmpegExtractTimeout)
	defer cancel()

	cmd := exec.CommandContext(cctx,
		"ffmpeg", "-nostdin", "-y",
		"-ss", "0", "-i", videoPath,
		"-frames:v", "1", "-q:v", "3",
		"-f", "image2", "-update", "1",
		tmpPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg first-frame extract: %w (output: %s)", err, string(out))
	}
	if err := os.Rename(tmpPath, outPath); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", tmpPath, outPath, err)
	}
	return nil
}

// nextFrameRefresher tracks the last-seen currently-playing video and
// triggers a re-extract whenever it changes. Mirrors the watchdog's
// goroutine shape (no libvlc event-listener wiring; just a poll).
type nextFrameRefresher struct {
	mu           sync.Mutex
	lastSeenPath string
}

// StartNextFrameRefresher launches a goroutine that polls
// currentlyPlaying() every interval and re-extracts the cover-frame
// cache when the playing video changes. Catches natural advances (libvlc
// internal) and any playAtIndex-driven cut uniformly.
func (s *Server) StartNextFrameRefresher(ctx context.Context, interval time.Duration) {
	r := &nextFrameRefresher{}
	go func() {
		slog.InfoContext(ctx, "starting next-frame refresher",
			"interval", interval,
			"cache", NextFrameCachePath(),
		)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.tick(ctx, s)
			}
		}
	}()
}

func (r *nextFrameRefresher) tick(ctx context.Context, s *Server) {
	current := s.currentlyPlaying()
	if current == "" {
		return
	}
	r.mu.Lock()
	changed := current != r.lastSeenPath
	r.mu.Unlock()
	if !changed {
		return
	}

	nextPath := s.NextVideoFile()
	if nextPath == "" {
		return
	}
	if !filepath.IsAbs(nextPath) {
		nextPath = filepath.Join(c.Conf.VideoDir, nextPath)
	}

	if err := ExtractFirstFrame(ctx, nextPath, NextFrameCachePath()); err != nil {
		slog.WarnContext(ctx, "failed to extract next-frame", "err", err, "video", nextPath)
		return
	}

	r.mu.Lock()
	r.lastSeenPath = current
	r.mu.Unlock()
	slog.InfoContext(ctx, "refreshed next-frame cover",
		"current", current,
		"next", filepath.Base(nextPath),
	)
}
