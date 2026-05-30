package onscreensClient

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/natsclient"
	oe "github.com/adanalife/tripbot/pkg/onscreens-events"
	"github.com/adanalife/tripbot/pkg/scoreboards"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Client talks to the onscreens-server HTTP API and mirrors each command
// onto NATS. Construct via New(host, nats, env).
//
// The NATS publish is the HTTP→NATS migration (phase 2): commands publish to
// NATS alongside the HTTP call, with HTTP still the source of truth, and the
// onscreens-server subscriber acts on the same payload. nats may be nil
// (tests that don't exercise pubsub) — publishes no-op then.
type Client struct {
	serverURL  string
	httpClient *http.Client
	nats       natsclient.Publisher
	env        string
}

// New returns a Client pointed at the given onscreens-server host. The HTTP
// transport is OTel-instrumented so outbound calls produce spans and
// propagate W3C tracecontext headers. nats + env drive the NATS mirror; pass
// natsclient.DefaultPublisher() in production, or a nil publisher to disable
// the mirror (tests).
func New(host string, nats natsclient.Publisher, env string) *Client {
	return &Client{
		serverURL:  "http://" + host,
		httpClient: &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)},
		nats:       nats,
		env:        env,
	}
}

// publish marshals ev and fires it on subject. Fire-and-forget: marshal
// errors are logged, and a nil publisher (or a nil underlying conn) no-ops.
func (c *Client) publish(ctx context.Context, subject string, ev any) {
	if c.nats == nil {
		return
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		slog.ErrorContext(ctx, "marshal onscreens event", "err", err, "subject", subject)
		return
	}
	c.nats.Publish(ctx, subject, payload)
}

func (c *Client) HideMiddleText(ctx context.Context) error {
	c.publish(ctx, oe.MiddleHideSubject(c.env), oe.Command{Envelope: oe.NewEnvelope()})
	_, err := c.get(ctx, c.serverURL+"/onscreens/middle/hide")
	if err != nil {
		slog.ErrorContext(ctx, "error hiding middle onscreen", "err", err)
		return err
	}
	return nil
}

func (c *Client) ShowMiddleText(ctx context.Context, msg string) error {
	// Publish NATS first (cheap, fire-and-forget) so a slow HTTP call doesn't
	// delay it; HTTP remains the source of truth during the mirror period.
	c.publish(ctx, oe.MiddleShowSubject(c.env), oe.MiddleShow{
		Envelope: oe.NewEnvelope(),
		Msg:      msg,
	})
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
	// onscreens-server renders the HTML now, so both transports carry the
	// structured {title, rows} payload. Build it once and reuse it for the
	// NATS publish and the base64-JSON HTTP query param.
	ev := oe.LeaderboardShow{
		Envelope: oe.NewEnvelope(),
		Title:    title,
		Rows:     leaderboard,
	}
	c.publish(ctx, oe.LeaderboardShowSubject(c.env), ev)

	payload, err := json.Marshal(ev)
	if err != nil {
		slog.ErrorContext(ctx, "marshal leaderboard event", "err", err)
		return err
	}
	url := c.serverURL + "/onscreens/leaderboard/show"
	url = fmt.Sprintf("%s?content=%s", url, helpers.Base64Encode(string(payload)))

	_, err = c.get(ctx, url)
	if err != nil {
		slog.ErrorContext(ctx, "error showing leaderboard onscreen", "err", err)
		return err
	}
	return nil
}

//TODO: this is taken right from the !guessleaderboard command, DRY it?
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
	c.publish(ctx, oe.TimewarpShowSubject(c.env), oe.Command{Envelope: oe.NewEnvelope()})
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
	// dur isn't transported — the server owns the GPS overlay's duration
	// (gpsDuration); the HTTP query param is vestigial and ignored there too.
	c.publish(ctx, oe.GPSShowSubject(c.env), oe.Command{Envelope: oe.NewEnvelope()})
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
	c.publish(ctx, oe.GPSHideSubject(c.env), oe.Command{Envelope: oe.NewEnvelope()})
	_, err := c.get(ctx, c.serverURL+"/onscreens/gps/hide")
	if err != nil {
		slog.ErrorContext(ctx, "error hiding gps onscreen", "err", err)
		return err
	}
	return nil
}

//TODO: move this to a common location
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
