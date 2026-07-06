package vlcServer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"path/filepath"
	"time"
)

// TODO: should we handle the case where index is outside range?
// or just explicitly pass in what we get here?
func (s *Server) playAtIndex(index int) error {
	// start playing the media. The underlying media player is primed with a
	// media at construction (see primePlayer) so this returns correctly even
	// when it's the very first play against the freshly-created list player.
	if err := s.Playlist.PlayAtIndex(uint(index)); err != nil {
		return err
	}
	// Every playback path (random / file / skip / back) funnels through here,
	// so this is the one spot that keeps the JetStream last-value cache
	// current for resume-on-restart. No-op when NATS is off.
	if index >= 0 && index < len(s.VideoFiles) {
		s.announceLastPlayed(s.VideoFiles[index])
	}
	return nil
}

// PlayVideoFile plays a video file in the playlist by basename. Returns
// an error if the file isn't in the loaded playlist. Exported so cmd
// callers (resume-from-marker on startup) can drive playback.
func (s *Server) PlayVideoFile(vidStr string) error {
	videoFile := filepath.Base(vidStr)
	index := s.getIndex(videoFile)
	if index < 0 {
		return fmt.Errorf("video file not in playlist: %s", videoFile)
	}
	return s.playAtIndex(index)
}

// PlayVideoFileAt plays a video file by basename and then seeks to positionMs
// within it. Same pair the resume-on-restart path runs (PlayVideoFile then
// SeekToPosition) — factored out so the play.at NATS handler and a future
// caller share one entry point. Seeking is async + best-effort (it waits for
// libvlc to reach Playing, then guards against the clip tail); positionMs 0
// just plays from the top. Returns the PlayVideoFile error if the file isn't
// in the playlist.
func (s *Server) PlayVideoFileAt(ctx context.Context, vidStr string, positionMs int64) error {
	if err := s.PlayVideoFile(vidStr); err != nil {
		return err
	}
	s.SeekToPosition(ctx, positionMs)
	return nil
}

// nextIndex computes a wrapped playlist position. Pure function so it can
// be unit-tested without libvlc. Negative offsets wrap to the back of the list.
func nextIndex(current, offset, length int) int {
	if length <= 0 {
		return 0
	}
	idx := (current + offset) % length
	if idx < 0 {
		idx += length
	}
	return idx
}

// skip plays the video n items forward in the playlist,
func (s *Server) skip(n int) error {
	return s.playAtIndex(nextIndex(s.currentIndex(), n, len(s.VideoFiles)))
}

// back plays the video n items backward in the playlist,
func (s *Server) back(n int) error {
	return s.playAtIndex(nextIndex(s.currentIndex(), -n, len(s.VideoFiles)))
}

// PlayRandom plays a random file from the playlist
func (s *Server) PlayRandom() error {
	count, err := s.MediaList.Count()
	if err != nil {
		slog.Error("error counting media in VLC media list", "err", err)
	}

	if count < 1 {
		err = errors.New("missing media")
		slog.Error("no media was found to play", "err", err)
		return err
	}

	random := rand.Intn(count)

	// start playing the media
	return s.playAtIndex(random)
}

// seekTailGuardMs keeps a resume-seek from landing in the last moments of a
// clip — seeking to (or past) the end would make the list player roll
// straight over to the next clip, which reads as a glitch, not a resume.
const seekTailGuardMs = 2000

// seekSettleTimeout bounds how long SeekToPosition waits for libvlc to reach
// Playing state before giving up. Clips load in well under a second locally;
// the headroom covers cold NFS reads.
const seekSettleTimeout = 5 * time.Second

// shouldSeekTo reports whether resuming at positionMs is worthwhile given the
// clip's length. Pure so the guard is unit-testable without libvlc. Unknown
// length (lengthMs <= 0, e.g. media not parsed yet) errs toward seeking —
// libvlc clamps overshoots.
func shouldSeekTo(positionMs, lengthMs int64) bool {
	if positionMs <= 0 {
		return false
	}
	if lengthMs > 0 && positionMs >= lengthMs-seekTailGuardMs {
		return false
	}
	return true
}

// SeekToPosition seeks the currently-loading clip to positionMs once libvlc
// actually reaches Playing state — a seek issued during Opening is silently
// ignored, so the wait is load-bearing. Runs async (resume happens during
// boot; blocking here would delay the HTTP listener) and is best-effort: on
// timeout or error playback simply continues from the top of the clip.
//
// Burn-in note: the clip is being re-streamed through the RTSP sout chain
// while we seek; the one-time discontinuity lands at boot, when the OBS
// ffmpeg_source is reconnecting anyway.
func (s *Server) SeekToPosition(ctx context.Context, positionMs int64) {
	// The upper bound rejects garbage input (no clip runs ~24.8 days) and
	// makes the int64→int conversion safe where int is 32 bits.
	if positionMs <= 0 || positionMs > math.MaxInt32 || s.Player == nil {
		return
	}
	seekToMs := int(positionMs)
	go func() {
		deadline := time.Now().Add(seekSettleTimeout)
		for !s.Player.IsPlaying() {
			if time.Now().After(deadline) {
				slog.WarnContext(ctx, "resume seek skipped: player never reached Playing", "position_ms", positionMs)
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(50 * time.Millisecond):
			}
		}
		// MediaLength can read 0 until the demuxer settles; it's only a
		// guard, so take whatever it says at this point.
		lengthMs, _ := s.Player.MediaLength()
		if !shouldSeekTo(positionMs, int64(lengthMs)) {
			slog.InfoContext(ctx, "resume seek skipped: position at clip tail",
				"position_ms", positionMs, "length_ms", lengthMs)
			return
		}
		if err := s.Player.SetMediaTime(seekToMs); err != nil {
			slog.WarnContext(ctx, "resume seek failed; continuing from clip start", "err", err, "position_ms", positionMs)
			return
		}
		slog.InfoContext(ctx, "resumed playback position", "position_ms", positionMs, "length_ms", lengthMs)
	}()
}

func (s *Server) getIndex(vidStr string) int {
	for i, file := range s.VideoFiles {
		if file == vidStr {
			return i
		}
	}
	return -1
}

func (s *Server) currentIndex() int {
	// extract just the filename
	videoFile := filepath.Base(s.currentlyPlaying())
	return s.getIndex(videoFile)
}

// NextVideoFile returns the basename of the next video in the playlist
// after the currently-playing one (with wrap). Pure helper for the
// cover-frame refresher; doesn't touch libvlc.
func (s *Server) NextVideoFile() string {
	if len(s.VideoFiles) == 0 {
		return ""
	}
	return s.VideoFiles[nextIndex(s.currentIndex(), +1, len(s.VideoFiles))]
}
