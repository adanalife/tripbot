package vlcServer

import (
	"errors"
	"log/slog"
	"math/rand"
	"path/filepath"
)

//TODO: should we handle the case where index is outside range?
// or just explicitly pass in what we get here?
func (s *Server) playAtIndex(index int) error {
	// start playing the media
	return s.Playlist.PlayAtIndex(uint(index))
}

// playVideoFile plays a video file in the playlist
func (s *Server) playVideoFile(vidStr string) error {
	// extract just the filename
	videoFile := filepath.Base(vidStr)
	index := s.getIndex(videoFile)
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
