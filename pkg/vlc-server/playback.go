package vlcServer

import (
	"math/rand"

	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
)

func playVideoFile(vidStr string) error {
	index := getIndex(vidStr)
	return playAtIndex(index)
}

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
