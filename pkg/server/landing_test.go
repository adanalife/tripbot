package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/video"
)

func TestSiblingURL(t *testing.T) {
	cases := []struct {
		in, service, want string
	}{
		{"https://tripbot.prod.whereisdana.today", "obs", "https://obs.prod.whereisdana.today"},
		{"https://tripbot.stage.whereisdana.today", "obs", "https://obs.stage.whereisdana.today"},
		{"http://localhost:8080", "obs", ""}, // not an FQDN — no sibling Ingress
		{"", "obs", ""},
	}
	for _, tc := range cases {
		if got := siblingURL(tc.in, tc.service); got != tc.want {
			t.Errorf("siblingURL(%q, %q) = %q, want %q", tc.in, tc.service, got, tc.want)
		}
	}
}

func TestLandingHandler_RendersReadyStatusAndLinks(t *testing.T) {
	defer SetReady(false)
	SetReady(true)

	// stand in for vlc-server: readiness ping + version endpoint
	vlc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health/ready":
			w.WriteHeader(http.StatusOK)
		case "/version":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"tag":"v9.9.9-vlc"}`))
		default:
			t.Errorf("unexpected vlc request %q", r.URL.Path)
		}
	}))
	defer vlc.Close()

	withConf(t, func() {
		c.Conf.VlcServerHost = strings.TrimPrefix(vlc.URL, "http://")
		c.Conf.ChannelName = "adanalife"
		c.Conf.ExternalURL = "https://tripbot.prod.whereisdana.today"
		c.Conf.Environment = "production"
	})
	withCurrentlyPlaying(t, video.Video{Slug: "wy_0042", State: "Wyoming"}, 3*time.Minute+12*time.Second)
	withChatterCount(t, 12)

	saved := versionTag
	defer func() { versionTag = saved }()
	SetVersion("v1.2.3")

	rec := httptest.NewRecorder()
	landingHandler(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"adanalife",                          // channel
		"v1.2.3",                             // tripbot build tag
		"ready",                              // tripbot status
		"healthy · v9.9.9-vlc",               // vlc status + fetched version tag
		"12 in chat",                         // chatter count
		"now playing",                        // now-playing section shown when vlc healthy
		"wy_0042.MP4",                        // current video file
		"Wyoming",                            // current video state
		"3m12s",                              // clip progress
		"https://obs.prod.whereisdana.today", // derived OBS link
		grafanaURL,                           // grafana link
		"https://twitch.tv/adanalife",        // twitch link
		githubURL,                            // github link
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}

func TestLandingHandler_DegradedAndVlcUnreachable(t *testing.T) {
	defer SetReady(false)
	SetReady(false)

	withConf(t, func() {
		// unroutable host → ping fails fast / times out → vlc shown unreachable
		c.Conf.VlcServerHost = "vlc-server.invalid:8080"
		c.Conf.ChannelName = "adanalife"
		c.Conf.ExternalURL = "https://tripbot.prod.whereisdana.today"
	})
	// even with a video loaded, an unhealthy vlc hides "now playing" rather
	// than showing a possibly-stale value.
	withCurrentlyPlaying(t, video.Video{Slug: "wy_0042", State: "Wyoming"}, time.Minute)

	rec := httptest.NewRecorder()
	landingHandler(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "degraded") {
		t.Errorf("body should report tripbot degraded; got %q", body)
	}
	if !strings.Contains(body, "unreachable") {
		t.Errorf("body should report vlc unreachable; got %q", body)
	}
	if strings.Contains(body, "now playing") {
		t.Errorf("now-playing should be hidden when vlc is unhealthy; got %q", body)
	}
}

// withConf snapshots the config fields the landing handler reads, runs set to
// mutate them for the test, and restores them afterward.
func withConf(t *testing.T, set func()) {
	t.Helper()
	saved := struct{ vlc, channel, external, env string }{
		c.Conf.VlcServerHost, c.Conf.ChannelName, c.Conf.ExternalURL, c.Conf.Environment,
	}
	t.Cleanup(func() {
		c.Conf.VlcServerHost = saved.vlc
		c.Conf.ChannelName = saved.channel
		c.Conf.ExternalURL = saved.external
		c.Conf.Environment = saved.env
	})
	set()
}

// withCurrentlyPlaying swaps the currentlyPlaying / currentProgress seams so
// the landing handler sees a fixed video + progress without driving the player.
func withCurrentlyPlaying(t *testing.T, v video.Video, progress time.Duration) {
	t.Helper()
	savedV, savedP := currentlyPlaying, currentProgress
	currentlyPlaying = func() video.Video { return v }
	currentProgress = func() time.Duration { return progress }
	t.Cleanup(func() { currentlyPlaying, currentProgress = savedV, savedP })
}

// withChatterCount swaps the chatterCount seam to a fixed value.
func withChatterCount(t *testing.T, n int) {
	t.Helper()
	saved := chatterCount
	chatterCount = func() int { return n }
	t.Cleanup(func() { chatterCount = saved })
}
