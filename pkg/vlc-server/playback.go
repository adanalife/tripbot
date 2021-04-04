package vlcServer

import (
	"crypto/rand"
	"errors"
	"path/filepath"

	terrors "github.com/adanalife/tripbot/pkg/errors"
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

// skip plays the video n items forward in the playlist,
func skip(n int) error {
	index := currentIndex() + n
	index = index % len(videoFiles)
	return playAtIndex(index)
}

// back plays the video n items backward in the playlist,
func back(n int) error {
	index := currentIndex() - n
	index = index % len(videoFiles)
	if index < 0 {
		// if we're negative, we have to find our spot at the back of the list
		index = len(videoFiles) + index
	}
	return playAtIndex(index)
}

// PlayRandom plays a random file from the playlist
func PlayRandom() error {
	count, err := mediaList.Count()
	if err != nil {
		terrors.Log(err, "error counting media in VLC media list")
	}

	if count < 1 {
		err = errors.New("missing media")
		terrors.Log(err, "no media was found to play")
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
