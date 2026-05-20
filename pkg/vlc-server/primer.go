package vlcServer

import (
	"context"
	"io"
	"log/slog"
	"math/rand"
	"os"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/vlc-server"
)

// The visible "flash" between clips is OBS's ffmpeg_source briefly
// disconnecting while libvlc opens the next file off the NAS — long
// enough that the Dashcam layer goes transparent and the scene
// background + "broken video" overlay show through. The dominant cost is
// NAS first-byte / metadata latency on a cold file open. Priming attacks
// that directly: read upcoming files into the kernel page cache ahead of
// time so libvlc's open hits warm pages instead of a cold round-trip.
//
// Two primers run off one poll loop:
//   - sequential: once the current clip passes VlcPrimePositionThreshold,
//     warm the next playlist file so the natural EOS transition is smooth.
//   - random: keep one pre-picked, warmed index ready so !timewarp jumps
//     to an already-warm file (PlayWarmRandom consumes it).

// warmCache reads a file into the kernel page cache. Bounded by limit
// bytes (0 = whole file). Fire-and-forget: errors are logged, not
// returned, since a failed warm just means the open is no faster than
// before — never worse.
func warmCache(ctx context.Context, path string, limit int64) {
	f, err := os.Open(path)
	if err != nil {
		slog.WarnContext(ctx, "prime: could not open file to warm cache", "file", path, "err", err)
		return
	}
	defer f.Close()

	var r io.Reader = f
	if limit > 0 {
		r = io.LimitReader(f, limit)
	}
	if _, err := io.Copy(io.Discard, r); err != nil {
		slog.WarnContext(ctx, "prime: error reading file to warm cache", "file", path, "err", err)
	}
}

// StartPrimer launches the priming goroutine. No-op when disabled or when
// there's nothing to prime (e.g. an empty corpus in tests).
func (s *Server) StartPrimer(ctx context.Context, interval time.Duration) {
	if !c.Conf.VlcPrimeEnabled {
		slog.InfoContext(ctx, "page-cache priming disabled (VLC_PRIME_ENABLED=false)")
		return
	}
	if len(s.VideoPaths) == 0 {
		slog.WarnContext(ctx, "page-cache priming: no video paths, not starting primer")
		return
	}
	s.primeCtx = ctx
	slog.InfoContext(ctx, "starting page-cache primer", "interval", interval, "videos", len(s.VideoPaths))
	go s.prime(ctx, interval)
}

func (s *Server) prime(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// warm an initial random so the first !timewarp is fast too.
	s.refreshWarmRandom(ctx)

	lastWarmedNext := -1
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if s.Player == nil {
				continue
			}
			// keep a warm random ready for the next !timewarp.
			s.ensureWarmRandom(ctx)

			pos, err := s.Player.MediaPosition()
			if err != nil {
				continue
			}
			if float64(pos) < c.Conf.VlcPrimePositionThreshold {
				continue
			}
			next := nextIndex(s.currentIndex(), 1, len(s.VideoPaths))
			if next < 0 || next >= len(s.VideoPaths) || next == lastWarmedNext {
				continue
			}
			lastWarmedNext = next
			go warmCache(ctx, s.VideoPaths[next], c.Conf.VlcPrimeBytes)
		}
	}
}

// ensureWarmRandom warms a fresh random index if none is currently ready.
func (s *Server) ensureWarmRandom(ctx context.Context) {
	s.primeMu.Lock()
	ready := s.warmRandomIdx >= 0
	s.primeMu.Unlock()
	if !ready {
		s.refreshWarmRandom(ctx)
	}
}

// refreshWarmRandom picks a new random index, records it, and warms it in
// the background.
func (s *Server) refreshWarmRandom(ctx context.Context) {
	n := len(s.VideoPaths)
	if n == 0 {
		return
	}
	idx := rand.Intn(n)
	s.primeMu.Lock()
	s.warmRandomIdx = idx
	s.primeMu.Unlock()
	go warmCache(ctx, s.VideoPaths[idx], c.Conf.VlcPrimeBytes)
}

// PlayWarmRandom plays the pre-warmed random index when one is ready,
// then kicks off warming a fresh one for next time. Falls back to a cold
// PlayRandom if no warm index is available (e.g. priming disabled, or a
// second !timewarp before the refresh completed).
func (s *Server) PlayWarmRandom() error {
	s.primeMu.Lock()
	idx := s.warmRandomIdx
	s.warmRandomIdx = -1
	s.primeMu.Unlock()

	if idx < 0 || idx >= len(s.VideoFiles) {
		return s.PlayRandom()
	}

	err := s.playAtIndex(idx)

	ctx := s.primeCtx
	if ctx == nil {
		ctx = context.Background()
	}
	go s.refreshWarmRandom(ctx)
	return err
}
