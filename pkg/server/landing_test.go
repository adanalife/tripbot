package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
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

	// stand in for vlc-server's readiness endpoint
	vlc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health/ready" {
			t.Errorf("vlc ping hit %q, want /health/ready", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer vlc.Close()

	withConf(t, func() {
		c.Conf.VlcServerHost = strings.TrimPrefix(vlc.URL, "http://")
		c.Conf.ChannelName = "adanalife"
		c.Conf.ExternalURL = "https://tripbot.prod.whereisdana.today"
		c.Conf.Environment = "production"
	})

	rec := httptest.NewRecorder()
	landingHandler(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"adanalife",                           // channel
		"ready",                               // tripbot status
		"healthy",                             // vlc status (ping succeeded)
		"https://obs.prod.whereisdana.today",  // derived OBS link
		grafanaURL,                            // grafana link
		"https://twitch.tv/adanalife",         // twitch link
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
