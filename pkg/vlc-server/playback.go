package vlcServer

import (
	"log/slog"
	"errors"
	"math/rand"
	"path/filepath"

)

//TODO: should we handle the case where index is outside range?
// or just explicitly pass in what we get here?
func playAtIndex(index int) error {
	// start playing the media
	return playlist.PlayAtIndex(uint(index))
}

// playVideoFile plays a video file in the playlist
func playVideoFile(vidStr string) error {
	// extract just the filename
	videoFile := filepath.Base(vidStr)
	index := getIndex(videoFile)
	return playAtIndex(index)
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
func skip(n int) error {
	return playAtIndex(nextIndex(currentIndex(), n, len(videoFiles)))
}

// back plays the video n items backward in the playlist,
func back(n int) error {
	return playAtIndex(nextIndex(currentIndex(), -n, len(videoFiles)))
}

// PlayRandom plays a random file from the playlist
func PlayRandom() error {
	count, err := mediaList.Count()
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
	return playAtIndex(random)
}

func getIndex(vidStr string) int {
	for i, file := range videoFiles {
		if file == vidStr {
			return i
		}
	}
	return -1
}

func currentIndex() int {
	// extract just the filename
	videoFile := filepath.Base(currentlyPlaying())
	return getIndex(videoFile)
}
