package vlcServer

import (
	"math/rand"
	"path/filepath"

	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
)

//TODO: handle the case where index is outside range
func back(n int) error {
	return playAtIndex(currentIndex() - n)
}

//TODO: handle the case where index is outside range
func skip(n int) error {
	return playAtIndex(currentIndex() + n)
}

func playVideoFile(vidStr string) error {
	// extract just the filename
	videoFile := filepath.Base(vidStr)
	index := getIndex(videoFile)
	return playAtIndex(index)
}

//TODO: handle the case where index is outside range
func playAtIndex(index int) error {
	// start playing the media
	return playlist.PlayAtIndex(uint(index))
}

// PlayRandom plays a random file from the playlist
func PlayRandom() error {
	count, err := mediaList.Count()
	if err != nil {
		terrors.Log(err, "error counting media in VLC media list")
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
	return getIndex(currentlyPlaying())
}
