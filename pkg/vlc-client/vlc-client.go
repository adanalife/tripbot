package vlcClient

import (
	"context"
	"fmt"
	"io/ioutil"
	"log/slog"
	"net/http"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

//TODO: eventually support HTTPS
var vlcServerURL = "http://" + c.Conf.VlcServerHost

// httpClient wraps the default transport with OpenTelemetry instrumentation
// so outbound calls produce spans and propagate W3C tracecontext headers.
// Callers must pass ctx so the propagation has an active span to attach to —
// passing context.Background() will still send the request, just without a
// parent span linking it to the caller's trace.
var httpClient = &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}

// CurrentlyPlaying finds the currently-playing video path
func CurrentlyPlaying(ctx context.Context) string {
	response, err := getUrl(ctx, vlcServerURL+"/vlc/current")
	if err != nil {
		slog.ErrorContext(ctx, "unable to determine current video", "err", err)
		return ""
	}
	return response
}

// PlayRandom plays a random file from the playlist
func PlayRandom(ctx context.Context) error {
	_, err := getUrl(ctx, vlcServerURL+"/vlc/random")
	if err != nil {
		slog.ErrorContext(ctx, "error playing random video", "err", err)
		return err
	}
	return nil
}

// PlayFileInPlaylist plays a given file
func PlayFileInPlaylist(ctx context.Context, filename string) error {
	url := vlcServerURL + "/vlc/play/" + filename
	_, err := getUrl(ctx, url)
	if err != nil {
		slog.ErrorContext(ctx, "error playing file", "err", err)
		return err
	}
	return nil
}

func Skip(ctx context.Context, n int) error {
	url := vlcServerURL + "/vlc/skip"
	if n > 0 {
		url = fmt.Sprintf("%s/%d", url, n)
	}
	_, err := getUrl(ctx, url)
	if err != nil {
		slog.ErrorContext(ctx, "error skipping video", "err", err)
		return err
	}
	return nil
}

func Back(ctx context.Context, n int) error {
	url := vlcServerURL + "/vlc/back"
	if n > 0 {
		url = fmt.Sprintf("%s/%d", url, n)
	}
	_, err := getUrl(ctx, url)
	if err != nil {
		slog.ErrorContext(ctx, "error going back to a video", "err", err)
		return err
	}
	return nil
}

//TODO: move this to a common location
func getUrl(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		slog.ErrorContext(ctx, "error building request to VLC server", "err", err)
		return "", err
	}
	response, err := httpClient.Do(req)
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
