package onscreensClient

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/adanalife/tripbot/pkg/config"
	terrors "github.com/adanalife/tripbot/pkg/errors"
)

var onscreensServerURL = "http://" + config.VlcServerHost

func SetMiddleText(msg string) error {
	url := onscreensServerURL + "/onscreens/middle/set"
	url = fmt.Sprintf("%s?msg=\"%s\"", url, msg)
	_, err := getUrl(url)
	if err != nil {
		terrors.Log(err, "error setting middle onscreen")
		return err
	}
	return nil
}

func HideMiddleText() error {
	_, err := getUrl(onscreensServerURL + "/onscreens/middle/hide")
	if err != nil {
		terrors.Log(err, "error showing leaderboard onscreen")
		return err
	}
	return nil
}

func ShowMiddleText(dur time.Duration) error {
	url := onscreensServerURL + "/onscreens/middle/show"
	url = fmt.Sprintf("%s?duration=\"%s\"", url, dur)
	_, err := getUrl(url)
	if err != nil {
		terrors.Log(err, "error showing middle onscreen")
		return err
	}
	return err
}

func ShowLeaderboard() error {
	_, err := getUrl(onscreensServerURL + "/onscreens/leaderboard/show")
	if err != nil {
		terrors.Log(err, "error showing leaderboard onscreen")
		return err
	}
	return nil
}

func ShowTimewarp() error {
	_, err := getUrl(onscreensServerURL + "/onscreens/timewarp/show")
	if err != nil {
		terrors.Log(err, "error showing timewarp onscreen")
		return err
	}
	return nil
}

func AddChatLine(username, msg string) error {
	url := onscreensServerURL + "/onscreens/chat/add"
	url = fmt.Sprintf("%s?msg=\"%s\"", url, msg)
	_, err := getUrl(url)
	if err != nil {
		terrors.Log(err, "error adding chat onscreen")
		return err
	}
	return nil
}

func ShowFlag(dur time.Duration) error {
	url := onscreensServerURL + "/onscreens/flag/show"
	url = fmt.Sprintf("%s?duration=\"%s\"", url, dur)
	_, err := getUrl(url)
	if err != nil {
		terrors.Log(err, "error showing flag onscreen")
		return err
	}
	return nil
}

func ShowGPSImage(dur time.Duration) error {
	url := onscreensServerURL + "/onscreens/gps/show"
	url = fmt.Sprintf("%s?duration=\"%s\"", url, dur)
	_, err := getUrl(url)
	if err != nil {
		terrors.Log(err, "error showing gps onscreen")
		return err
	}
	return nil
}

func HideGPSImage() error {
	_, err := getUrl(onscreensServerURL + "/onscreens/gps/hide")
	if err != nil {
		terrors.Log(err, "error hiding gps onscreen")
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
