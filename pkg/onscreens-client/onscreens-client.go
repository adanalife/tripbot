package onscreensClient

import (
	"log/slog"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/scoreboards"
	"github.com/adanalife/tripbot/pkg/users"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

var onscreensServerURL = "http://" + c.Conf.OnscreensServerHost

// httpClient wraps the default transport with OpenTelemetry instrumentation
// so outbound calls produce spans. See pkg/vlc-client for the same pattern.
var httpClient = &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}

func HideMiddleText() error {
	_, err := getUrl(onscreensServerURL + "/onscreens/middle/hide")
	if err != nil {
		slog.Error("error showing leaderboard onscreen", "err", err)
		return err
	}
	return nil
}

func ShowMiddleText(msg string) error {
	url := onscreensServerURL + "/onscreens/middle/show"
	url = fmt.Sprintf("%s?msg=%s", url, helpers.Base64Encode(msg))
	_, err := getUrl(url)
	if err != nil {
		slog.Error("error showing middle onscreen", "err", err)
		return err
	}
	return err
}

func ShowLeaderboard(title string, leaderboard [][]string) error {
	content := users.LeaderboardContent(title, leaderboard)

	url := onscreensServerURL + "/onscreens/leaderboard/show"
	url = fmt.Sprintf("%s?content=%s", url, helpers.Base64Encode(content))

	_, err := getUrl(url)
	if err != nil {
		slog.Error("error showing leaderboard onscreen", "err", err)
		return err
	}
	return nil
}

//TODO: this is taken right from the !guessleaderboard command, DRY it?
func ShowGuessLeaderboard(ctx context.Context) {
	// select users to show in leaderboard
	size := 10
	leaderboard := scoreboards.TopUsers(ctx, scoreboards.CurrentGuessScoreboard(), size)
	if size > len(leaderboard) {
		size = len(leaderboard)
	}
	leaderboard = leaderboard[:size]

	var intLeaderboard [][]string
	for _, leaderPair := range leaderboard {
		// guesses are ints not floats, so remove the decimal place
		intVersion := strings.Split(leaderPair[1], ".")[0]
		intLeaderboard = append(intLeaderboard, []string{leaderPair[0], intVersion})
	}

	// display leaderboard on screen
	ShowLeaderboard("Correct Guesses This Month", intLeaderboard)
}

func ShowTimewarp() error {
	_, err := getUrl(onscreensServerURL + "/onscreens/timewarp/show")
	if err != nil {
		slog.Error("error showing timewarp onscreen", "err", err)
		return err
	}
	return nil
}

func ShowFlag(dur time.Duration) error {
	//TODO: bring this back
	// url := onscreensServerURL + "/onscreens/flag/show"
	// url = fmt.Sprintf("%s?duration=%s", url, helpers.Base64Encode(string(rune(dur))))
	// _, err := getUrl(url)
	// if err != nil {
	// 	slog.Error("error showing flag onscreen", "err", err)
	// 	return err
	// }
	return nil
}

func ShowGPSImage(dur time.Duration) error {
	url := onscreensServerURL + "/onscreens/gps/show"
	url = fmt.Sprintf("%s?duration=%s", url, helpers.Base64Encode(string(rune(dur))))
	_, err := getUrl(url)
	if err != nil {
		slog.Error("error showing gps onscreen", "err", err)
		return err
	}
	return nil
}

func HideGPSImage() error {
	_, err := getUrl(onscreensServerURL + "/onscreens/gps/hide")
	if err != nil {
		slog.Error("error hiding gps onscreen", "err", err)
		return err
	}
	return nil
}

//TODO: move this to a common location
func getUrl(url string) (string, error) {
	response, err := httpClient.Get(url)
	if err != nil {
		slog.Error("error connecting to VLC server", "err", err)
		return "", err
	} else {
		defer response.Body.Close()
		contents, err := ioutil.ReadAll(response.Body)
		if err != nil {
			slog.Error("error reading response from VLC server", "err", err)
			return "", err
		}
		// make note of non-200 status codes
		if response.StatusCode != 200 {
			slog.Error(fmt.Sprintf("non-200 response from server (%d)", response.StatusCode))
		}
		return string(contents), nil
	}
}
