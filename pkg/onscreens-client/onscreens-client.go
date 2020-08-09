package onscreensClient

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/adanalife/tripbot/pkg/config"
	terrors "github.com/adanalife/tripbot/pkg/errors"
)

//TODO: eventually support HTTPS
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

func ShowLeaderboard() error {
	_, err := getUrl(onscreensServerURL + "/onscreens/leaderboard/show")
	if err != nil {
		terrors.Log(err, "error showing leaderboard onscreen")
		return err
	}
	return nil
}

func ShowTimewarp() string {
	_, err := getUrl(onscreensServerURL + "/onscreens/timewarp/show")
	if err != nil {
		terrors.Log(err, "error showing timewarp onscreen")
		return err
	}
	return nil
}

func AddChatLine(username, msg string) string {
	return "TODO"
}

func ShowFlag(time.Duration) string {
	return "TODO"
}

func ShowGPSImage(time.Duration) string {
	return "TODO"
}

func HideGPSImage() string {
	return "TODO"
}

func HideMiddleText() string {
	return "TODO"
}

func ShowMiddleText(msg string) string {
	return "TODO"
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
