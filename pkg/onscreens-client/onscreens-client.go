package onscreensClient

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/adanalife/tripbot/pkg/config"
	terrors "github.com/adanalife/tripbot/pkg/errors"
)

//TODO: add these
// AddChatLine(username, msg.Message)
// FlagShowFor("", 10*time.Second)
// GPSClientShowFor("", 60*time.Second)

//TODO: eventually support HTTPS
var onscreensServerURL = "http://" + config.VlcServerHost

func SetMiddleText(msg string) string {
	url := onscreensServerURL + "/onscreens/middle"
	url = fmt.Sprintf("%s?msg=\"%s\"", url, msg)
	_, err := getUrl(url)
	if err != nil {
		terrors.Log(err, "error setting middle onscreen")
		return err
	}
	return nil
}

func ShowLeaderboard() string {
	return "TODO"
}

func ShowTimewarp() string {
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
