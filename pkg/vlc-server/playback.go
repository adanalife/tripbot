package vlcServer

import (
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"path/filepath"
)

// TODO: should we handle the case where index is outside range?
// or just explicitly pass in what we get here?
func (s *Server) playAtIndex(index int) error {
	// start playing the media
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
