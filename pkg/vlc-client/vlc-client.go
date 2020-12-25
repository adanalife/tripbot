package vlcClient

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/adanalife/tripbot/pkg/config"
	terrors "github.com/adanalife/tripbot/pkg/errors"
)

//TODO: eventually support HTTPS
var vlcServerURL = "http://" + config.VlcServerHost

// CurrentlyPlaying finds the currently-playing video path
func CurrentlyPlaying() string {
	response, err := getUrl(vlcServerURL + "/vlc/current")
	if err != nil {
		terrors.Log(err, "unable to determine current video")
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

// PlayFileInPlaylist plays a given file
func PlayFileInPlaylist(filename string) error {
	url := vlcServerURL + "/vlc/play/" + filename
	_, err := getUrl(url)
	if err != nil {
		terrors.Log(err, "error playing file")
		return err
	}
	return nil
}

func Skip(n int) error {
	url := vlcServerURL + "/vlc/skip"
	if n > 0 {
		url = fmt.Sprintf("%s/%d", url, n)
	}
	_, err := getUrl(url)
	if err != nil {
		terrors.Log(err, "error skipping video")
		return err
	}
	return nil
}

func Back(n int) error {
	url := vlcServerURL + "/vlc/back"
	if n > 0 {
		url = fmt.Sprintf("%s/%d", url, n)
	}
	_, err := getUrl(url)
	if err != nil {
		terrors.Log(err, "error going back to a video")
		return err
	}
	return nil
}

//TODO: move this to a common location
func getUrl(url string) (string, error) {
	response, err := http.Get(url)
	if err != nil {
		terrors.Log(err, "error connecting to VLC server")
		return "", err
	} else {
		defer response.Body.Close()
		contents, err := ioutil.ReadAll(response.Body)
		if err != nil {
			terrors.Log(err, "error reading response from VLC server")
			return "", err
		}
		return string(contents), nil
	}
}
