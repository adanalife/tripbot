package onscreensClient

import (
	"context"
	"fmt"
	"io/ioutil"
	"log/slog"
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
// so outbound calls produce spans and propagate W3C tracecontext headers.
// See pkg/vlc-client for the same pattern.
var httpClient = &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}

func HideMiddleText(ctx context.Context) error {
	_, err := getUrl(ctx, onscreensServerURL+"/onscreens/middle/hide")
	if err != nil {
		slog.ErrorContext(ctx, "error hiding middle onscreen", "err", err)
		return err
	}
	return nil
}

func ShowMiddleText(ctx context.Context, msg string) error {
	url := onscreensServerURL + "/onscreens/middle/show"
	url = fmt.Sprintf("%s?msg=%s", url, helpers.Base64Encode(msg))
	_, err := getUrl(ctx, url)
	if err != nil {
		slog.ErrorContext(ctx, "error showing middle onscreen", "err", err)
		return err
	}
	return err
}

func ShowLeaderboard(ctx context.Context, title string, leaderboard [][]string) error {
	content := users.LeaderboardContent(title, leaderboard)

	url := onscreensServerURL + "/onscreens/leaderboard/show"
	url = fmt.Sprintf("%s?content=%s", url, helpers.Base64Encode(content))

	_, err := getUrl(ctx, url)
	if err != nil {
		slog.ErrorContext(ctx, "error showing leaderboard onscreen", "err", err)
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
	ShowLeaderboard(ctx, "Correct Guesses This Month", intLeaderboard)
}

func ShowTimewarp(ctx context.Context) error {
	_, err := getUrl(ctx, onscreensServerURL+"/onscreens/timewarp/show")
	if err != nil {
		slog.ErrorContext(ctx, "error showing timewarp onscreen", "err", err)
		return err
	}
	return nil
}

func ShowFlag(ctx context.Context, dur time.Duration) error {
	//TODO: bring this back
	// url := onscreensServerURL + "/onscreens/flag/show"
	// url = fmt.Sprintf("%s?duration=%s", url, helpers.Base64Encode(string(rune(dur))))
	// _, err := getUrl(ctx, url)
	// if err != nil {
	// 	slog.ErrorContext(ctx, "error showing flag onscreen", "err", err)
	// 	return err
	// }
	return nil
}

func ShowGPSImage(ctx context.Context, dur time.Duration) error {
	url := onscreensServerURL + "/onscreens/gps/show"
	url = fmt.Sprintf("%s?duration=%s", url, helpers.Base64Encode(string(rune(dur))))
	_, err := getUrl(ctx, url)
	if err != nil {
		slog.ErrorContext(ctx, "error showing gps onscreen", "err", err)
		return err
	}
	return nil
}

func HideGPSImage(ctx context.Context) error {
	_, err := getUrl(ctx, onscreensServerURL+"/onscreens/gps/hide")
	if err != nil {
		slog.ErrorContext(ctx, "error hiding gps onscreen", "err", err)
		return err
	}
	return nil
}

//TODO: move this to a common location
func getUrl(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		slog.ErrorContext(ctx, "error building request to onscreens server", "err", err)
		return "", err
	}
	response, err := httpClient.Do(req)
	if err != nil {
		slog.ErrorContext(ctx, "error connecting to onscreens server", "err", err)
		return "", err
	}
	defer response.Body.Close()
	contents, err := ioutil.ReadAll(response.Body)
	if err != nil {
		slog.ErrorContext(ctx, "error reading response from onscreens server", "err", err)
		return "", err
	}
	// make note of non-200 status codes
	if response.StatusCode != 200 {
		slog.ErrorContext(ctx, "non-200 response from server", "status", response.StatusCode)
	}
	return string(contents), nil
}
