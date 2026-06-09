package vlcClient

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log/slog"
	"net/http"

	"github.com/adanalife/tripbot/pkg/natsclient"
	ve "github.com/adanalife/tripbot/pkg/vlc-events"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Client talks to the vlc-server HTTP API and mirrors each fire-and-forget
// command onto NATS. Construct via New(host, nats, env).
//
// The NATS publish is the HTTP→NATS migration (observe-only mirror phase):
// the four playback commands publish to NATS alongside the HTTP call, with
// HTTP still the source of truth. vlc-server's subscriber currently only logs
// the published commands (VLC commands aren't idempotent, so it can't act
// while HTTP also acts). nats may be nil (tests that don't exercise pubsub) —
// publishes no-op then. CurrentlyPlaying is a read and stays HTTP-only.
type Client struct {
	serverURL  string
	httpClient *http.Client
	nats       natsclient.Publisher
	env        string
}

// New returns a Client pointed at the given vlc-server host. The HTTP
// transport is OTel-instrumented so outbound calls produce spans and
// propagate W3C tracecontext headers. Callers must pass ctx so the
// propagation has an active span to attach to — passing context.Background()
// will still send the request, just without a parent span linking it to the
// caller's trace. nats + env drive the NATS mirror; pass
// natsclient.DefaultPublisher() in production, or a nil publisher to disable
// the mirror (tests).
//
// TODO: eventually support HTTPS
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
		slog.ErrorContext(ctx, "marshal vlc event", "err", err, "subject", subject)
		return
	}
	c.nats.Publish(ctx, subject, payload)
}

// CurrentlyPlaying finds the currently-playing video path
func (c *Client) CurrentlyPlaying(ctx context.Context) string {
	response, err := c.get(ctx, c.serverURL+"/vlc/current")
	if err != nil {
		slog.ErrorContext(ctx, "unable to determine current video", "err", err)
		return ""
	}
	return response
}

// PlayRandom plays a random file from the playlist
func (c *Client) PlayRandom(ctx context.Context) error {
	c.publish(ctx, ve.PlayRandomSubject(c.env), ve.Command{Envelope: ve.NewEnvelope()})
	_, err := c.get(ctx, c.serverURL+"/vlc/random")
	if err != nil {
		slog.ErrorContext(ctx, "error playing random video", "err", err)
		return err
	}
	return nil
}

// PlayFileInPlaylist plays a given file
func (c *Client) PlayFileInPlaylist(ctx context.Context, filename string) error {
	c.publish(ctx, ve.PlayFileSubject(c.env), ve.PlayFile{Envelope: ve.NewEnvelope(), File: filename})
	url := c.serverURL + "/vlc/play/" + filename
	_, err := c.get(ctx, url)
	if err != nil {
		slog.ErrorContext(ctx, "error playing file", "err", err)
		return err
	}
	return nil
}

func (c *Client) Skip(ctx context.Context, n int) error {
	c.publish(ctx, ve.SkipSubject(c.env), ve.Skip{Envelope: ve.NewEnvelope(), N: n})
	url := c.serverURL + "/vlc/skip"
	if n > 0 {
		url = fmt.Sprintf("%s/%d", url, n)
	}
	_, err := c.get(ctx, url)
	if err != nil {
		slog.ErrorContext(ctx, "error skipping video", "err", err)
		return err
	}
	return nil
}

func (c *Client) Back(ctx context.Context, n int) error {
	c.publish(ctx, ve.BackSubject(c.env), ve.Back{Envelope: ve.NewEnvelope(), N: n})
	url := c.serverURL + "/vlc/back"
	if n > 0 {
		url = fmt.Sprintf("%s/%d", url, n)
	}
	_, err := c.get(ctx, url)
	if err != nil {
		slog.ErrorContext(ctx, "error going back to a video", "err", err)
		return err
	}
	return nil
}

// TODO: move this to a common location
//
// Transport-layer errors log at Debug, not Error: each wrapper above this
// (CurrentlyPlaying, PlayRandom, …) logs the operation-specific failure at
// Error with the same underlying err. Logging here too would triple-count
// every VLC outage in Loki and Sentry.
func (c *Client) get(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		slog.DebugContext(ctx, "error building request to VLC server", "err", err)
		return "", err
	}
	response, err := c.httpClient.Do(req)
	if err != nil {
		slog.DebugContext(ctx, "error connecting to VLC server", "err", err)
		return "", err
	}
	defer response.Body.Close()
	contents, err := ioutil.ReadAll(response.Body)
	if err != nil {
		slog.DebugContext(ctx, "error reading response from VLC server", "err", err)
		return "", err
	}
	return string(contents), nil
}
