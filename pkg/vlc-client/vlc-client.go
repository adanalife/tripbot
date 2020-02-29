package vlcClient

import (
	"io/ioutil"
	"net/http"

	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
)

//TODO: make this a Config value
const vlcServerURL = "http://localhost:8088"

// CurrentlyPlaying finds the currently-playing video path
func CurrentlyPlaying() string {
	response, err := getUrl(vlcServerURL + "/vlc/current")
	if err != nil {
		terrors.Log(err, "error getting current video")
		return ""
	}
	return response
}

// PlayRandom plays a random file from the playlist
func PlayRandom() error {
	_, err := getUrl(vlcServerURL + "/vlc/random")
	if err != nil {
		terrors.Log(err, "error playing random video")
		return err
	}
	return nil
}

func getUrl(url string) (string, error) {
	response, err := http.Get(url)
	if err != nil {
		terrors.Log(err, "error fetching URL")
		return "", err
	} else {
		defer response.Body.Close()
		contents, err := ioutil.ReadAll(response.Body)
		if err != nil {
			terrors.Log(err, "error reading reponse body")
			return "", err
		}
		return string(contents), nil
	}
}
