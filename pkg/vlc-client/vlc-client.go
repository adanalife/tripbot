package vlcClient

import (
	"context"
	"fmt"
	"io/ioutil"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Client talks to the vlc-server HTTP API. Construct via New(host).
type Client struct {
	serverURL  string
	httpClient *http.Client
}

// New returns a Client pointed at the given vlc-server host. The HTTP
// transport is OTel-instrumented so outbound calls produce spans and
// propagate W3C tracecontext headers. Callers must pass ctx so the
// propagation has an active span to attach to — passing context.Background()
// will still send the request, just without a parent span linking it to the
// caller's trace.
//
//TODO: eventually support HTTPS
func New(host string) *Client {
	return &Client{
		serverURL:  "http://" + host,
		httpClient: &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)},
	}
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
	_, err := c.get(ctx, c.serverURL+"/vlc/random")
	if err != nil {
		slog.ErrorContext(ctx, "error playing random video", "err", err)
		return err
	}
	return nil
}

// PlayFileInPlaylist plays a given file
func (c *Client) PlayFileInPlaylist(ctx context.Context, filename string) error {
	url := c.serverURL + "/vlc/play/" + filename
	_, err := c.get(ctx, url)
	if err != nil {
		slog.ErrorContext(ctx, "error playing file", "err", err)
		return err
	}
	return nil
}

func (c *Client) Skip(ctx context.Context, n int) error {
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

//TODO: move this to a common location
func (c *Client) get(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		slog.ErrorContext(ctx, "error building request to VLC server", "err", err)
		return "", err
	}
	response, err := c.httpClient.Do(req)
	if err != nil {
		slog.ErrorContext(ctx, "error connecting to VLC server", "err", err)
		return "", err
	}
	defer response.Body.Close()
	contents, err := ioutil.ReadAll(response.Body)
	if err != nil {
		slog.ErrorContext(ctx, "error reading response from VLC server", "err", err)
		return "", err
	}
	return string(contents), nil
}
