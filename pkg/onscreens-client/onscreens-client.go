package onscreensClient

import (
	"context"
	"fmt"
	"io/ioutil"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/scoreboards"
	"github.com/adanalife/tripbot/pkg/users"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Client talks to the onscreens-server HTTP API. Construct via New(host).
type Client struct {
	serverURL  string
	httpClient *http.Client
}

// New returns a Client pointed at the given onscreens-server host. The HTTP
// transport is OTel-instrumented so outbound calls produce spans and
// propagate W3C tracecontext headers.
func New(host string) *Client {
	return &Client{
		serverURL:  "http://" + host,
		httpClient: &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)},
	}
}

func (c *Client) HideMiddleText(ctx context.Context) error {
	_, err := c.get(ctx, c.serverURL+"/onscreens/middle/hide")
	if err != nil {
		slog.ErrorContext(ctx, "error hiding middle onscreen", "err", err)
		return err
	}
	return nil
}

func (c *Client) ShowMiddleText(ctx context.Context, msg string) error {
	url := c.serverURL + "/onscreens/middle/show"
	url = fmt.Sprintf("%s?msg=%s", url, helpers.Base64Encode(msg))
	_, err := c.get(ctx, url)
	if err != nil {
		slog.ErrorContext(ctx, "error showing middle onscreen", "err", err)
		return err
	}
	return err
}

func (c *Client) ShowLeaderboard(ctx context.Context, title string, leaderboard [][]string) error {
	content := users.LeaderboardContent(title, leaderboard)

	url := c.serverURL + "/onscreens/leaderboard/show"
	url = fmt.Sprintf("%s?content=%s", url, helpers.Base64Encode(content))

	_, err := c.get(ctx, url)
	if err != nil {
		slog.ErrorContext(ctx, "error showing leaderboard onscreen", "err", err)
		return err
	}
	return nil
}

// TODO: this is taken right from the !guessleaderboard command, DRY it?
func (c *Client) ShowGuessLeaderboard(ctx context.Context) {
	// select users to show in leaderboard
	size := 10
	leaderboard := scoreboards.TopUsers(ctx, scoreboards.CurrentGuessScoreboard(), size)
	if size > len(leaderboard) {
		size = len(leaderboard)
	}
	leaderboard = leaderboard[:size]

	// Filter zero-scorers (AddToScoreByName uses FirstOrCreate, so every
	// user who's ever guessed has a row — many at 0 early in the month).
	// If the filtered list is empty, skip the overlay entirely.
	var intLeaderboard [][]string
	for _, leaderPair := range leaderboard {
		// guesses are ints not floats, so remove the decimal place
		intVersion := strings.Split(leaderPair[1], ".")[0]
		if intVersion == "0" || intVersion == "" {
			continue
		}
		intLeaderboard = append(intLeaderboard, []string{leaderPair[0], intVersion})
	}
	if len(intLeaderboard) == 0 {
		return
	}

	// display leaderboard on screen
	c.ShowLeaderboard(ctx, "Correct Guesses This Month", intLeaderboard)
}

func (c *Client) ShowTimewarp(ctx context.Context) error {
	_, err := c.get(ctx, c.serverURL+"/onscreens/timewarp/show")
	if err != nil {
		slog.ErrorContext(ctx, "error showing timewarp onscreen", "err", err)
		return err
	}
	return nil
}

func (c *Client) ShowFlag(ctx context.Context, dur time.Duration) error {
	//TODO: bring this back
	// url := c.serverURL + "/onscreens/flag/show"
	// url = fmt.Sprintf("%s?duration=%s", url, helpers.Base64Encode(string(rune(dur))))
	// _, err := c.get(ctx, url)
	// if err != nil {
	// 	slog.ErrorContext(ctx, "error showing flag onscreen", "err", err)
	// 	return err
	// }
	return nil
}

func (c *Client) ShowGPSImage(ctx context.Context, dur time.Duration) error {
	url := c.serverURL + "/onscreens/gps/show"
	url = fmt.Sprintf("%s?duration=%s", url, helpers.Base64Encode(string(rune(dur))))
	_, err := c.get(ctx, url)
	if err != nil {
		slog.ErrorContext(ctx, "error showing gps onscreen", "err", err)
		return err
	}
	return nil
}

func (c *Client) HideGPSImage(ctx context.Context) error {
	_, err := c.get(ctx, c.serverURL+"/onscreens/gps/hide")
	if err != nil {
		slog.ErrorContext(ctx, "error hiding gps onscreen", "err", err)
		return err
	}
	return nil
}

// TODO: move this to a common location
//
// Transport-layer errors log at Debug, not Error: each wrapper above this
// (HideMiddleText, ShowGPSImage, …) logs the operation-specific failure at
// Error with the same underlying err. Logging here too would double-count
// every onscreens outage in Loki and Sentry.
func (c *Client) get(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		slog.DebugContext(ctx, "error building request to onscreens server", "err", err)
		return "", err
	}
	response, err := c.httpClient.Do(req)
	if err != nil {
		slog.DebugContext(ctx, "error connecting to onscreens server", "err", err)
		return "", err
	}
	defer response.Body.Close()
	contents, err := ioutil.ReadAll(response.Body)
	if err != nil {
		slog.DebugContext(ctx, "error reading response from onscreens server", "err", err)
		return "", err
	}
	// make note of non-200 status codes
	if response.StatusCode != 200 {
		slog.ErrorContext(ctx, "non-200 response from server", "status", response.StatusCode)
	}
	return string(contents), nil
}
