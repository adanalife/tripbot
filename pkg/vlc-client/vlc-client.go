package vlcClient

import (
	"log/slog"
	"fmt"
	"io/ioutil"
	"net/http"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

//TODO: eventually support HTTPS
var vlcServerURL = "http://" + c.Conf.VlcServerHost

// httpClient wraps the default transport with OpenTelemetry instrumentation
// so outbound calls produce spans. Most callers here are cron/IRC-driven
// (no inbound HTTP context), so traceparent propagation is best-effort
// until ctx-threaded variants of these helpers exist.
var httpClient = &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}

// CurrentlyPlaying finds the currently-playing video path
func CurrentlyPlaying() string {
	response, err := getUrl(vlcServerURL + "/vlc/current")
	if err != nil {
		slog.Error("unable to determine current video", "err", err)
		return ""
	}
	return response
}

// PlayRandom plays a random file from the playlist
func PlayRandom() error {
	_, err := getUrl(vlcServerURL + "/vlc/random")
	if err != nil {
		slog.Error("error playing random video", "err", err)
		return err
	}
	return nil
}

// PlayFileInPlaylist plays a given file
func PlayFileInPlaylist(filename string) error {
	url := vlcServerURL + "/vlc/play/" + filename
	_, err := getUrl(url)
	if err != nil {
		slog.Error("error playing file", "err", err)
		return err
	}
	return nil
}

func Skip(n int) error {
	url := vlcServerURL + "/vlc/skip"
	if n > 0 {
		url = fmt.Sprintf("%s/%d", url, n)
	}
	_, err := getUrl(url)
	if err != nil {
		slog.Error("error skipping video", "err", err)
		return err
	}
	return nil
}

func Back(n int) error {
	url := vlcServerURL + "/vlc/back"
	if n > 0 {
		url = fmt.Sprintf("%s/%d", url, n)
	}
	_, err := getUrl(url)
	if err != nil {
		slog.Error("error going back to a video", "err", err)
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
		return string(contents), nil
	}
}
