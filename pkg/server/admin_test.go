package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
	"github.com/adanalife/tripbot/pkg/video"
	"github.com/gorilla/mux"
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

func TestHubbleNamespace(t *testing.T) {
	saved := c.Conf.Environment
	t.Cleanup(func() { c.Conf.Environment = saved })
	cases := map[string]string{
		"production":  "prod-1",
		"staging":     "stage-1",
		"development": "stage-1", // shares ENV=staging; link is moot on its cluster
	}
	for env, want := range cases {
		c.Conf.Environment = env
		if got := hubbleNamespace(); got != want {
			t.Errorf("hubbleNamespace() with ENV=%q = %q, want %q", env, got, want)
		}
	}
}

func TestChangelogURL(t *testing.T) {
	if got, want := changelogURL("deadbeef"), githubURL+"/blob/deadbeef/CHANGELOG.md"; got != want {
		t.Errorf("changelogURL(sha) = %q, want %q", got, want)
	}
	if got, want := changelogURL(""), githubURL+"/blob/master/CHANGELOG.md"; got != want {
		t.Errorf("changelogURL(\"\") = %q, want %q", got, want)
	}
}

// withObsStream swaps the obsStartStream / obsStopStream / obsStreamStatus
// seams to test fakes so we can exercise the handler without an OBS
// WebSocket. Returns the captured invocation counts.
func withObsStream(t *testing.T, startErr, stopErr error) (started, stopped *int) {
	t.Helper()
	savedStart, savedStop := obsStartStream, obsStopStream
	t.Cleanup(func() { obsStartStream, obsStopStream = savedStart, savedStop })
	started, stopped = new(int), new(int)
	obsStartStream = func(context.Context) error { *started++; return startErr }
	obsStopStream = func(context.Context) error { *stopped++; return stopErr }
	return started, stopped
}

func TestObsStreamActionHandler_StartRedirectsAndCalls(t *testing.T) {
	started, _ := withObsStream(t, nil, nil)

	r := mux.NewRouter()
	r.Handle("/admin/obs/stream/{action}", http.HandlerFunc(obsStreamActionHandler)).Methods("POST")

	req := httptest.NewRequest(http.MethodPost, "/admin/obs/stream/start", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got := rec.Header().Get("Location"); got != "/" {
		t.Fatalf("got Location %q, want /", got)
	}
	if *started != 1 {
		t.Fatalf("obsStartStream called %d times, want 1", *started)
	}
}

func TestObsStreamActionHandler_StopRedirectsAndCalls(t *testing.T) {
	_, stopped := withObsStream(t, nil, nil)

	r := mux.NewRouter()
	r.Handle("/admin/obs/stream/{action}", http.HandlerFunc(obsStreamActionHandler)).Methods("POST")

	req := httptest.NewRequest(http.MethodPost, "/admin/obs/stream/stop", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if *stopped != 1 {
		t.Fatalf("obsStopStream called %d times, want 1", *stopped)
	}
}

func TestObsStreamActionHandler_UnknownActionIs400(t *testing.T) {
	withObsStream(t, nil, nil)

	r := mux.NewRouter()
	r.Handle("/admin/obs/stream/{action}", http.HandlerFunc(obsStreamActionHandler)).Methods("POST")

	req := httptest.NewRequest(http.MethodPost, "/admin/obs/stream/bogus", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestObsStreamActionHandler_RedirectsEvenOnError(t *testing.T) {
	// State is the source of truth — refreshed panel will show the actual
	// state. Surfacing the error in flash UI isn't worth the complexity for
	// a tailnet-only solo-operator panel.
	started, _ := withObsStream(t, errFakeOBS, nil)

	r := mux.NewRouter()
	r.Handle("/admin/obs/stream/{action}", http.HandlerFunc(obsStreamActionHandler)).Methods("POST")

	req := httptest.NewRequest(http.MethodPost, "/admin/obs/stream/start", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if *started != 1 {
		t.Fatalf("obsStartStream called %d times, want 1", *started)
	}
}

var errFakeOBS = errors.New("fake OBS error")

// withRestart swaps the restart seams so handler tests don't open real
// sockets or SIGTERM the test process. Returns the captured invocation
// records for assertion.
func withRestart(t *testing.T) (selfCalls *int, proxyHosts *[]string) {
	t.Helper()
	savedSelf, savedProxy := restartSelf, restartProxyShutdown
	t.Cleanup(func() { restartSelf, restartProxyShutdown = savedSelf, savedProxy })

	selfCalls, proxyHosts = new(int), new([]string)
	restartSelf = func() error { *selfCalls++; return nil }
	restartProxyShutdown = func(_ context.Context, host string) error {
		*proxyHosts = append(*proxyHosts, host)
		return nil
	}
	return selfCalls, proxyHosts
}

func TestRestartActionHandler_TripbotCallsSelf(t *testing.T) {
	selfCalls, proxyHosts := withRestart(t)

	r := mux.NewRouter()
	r.Handle("/admin/restart/{service}", http.HandlerFunc(restartActionHandler)).Methods("POST")
	req := httptest.NewRequest(http.MethodPost, "/admin/restart/tripbot", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if *selfCalls != 1 {
		t.Fatalf("restartSelf called %d times, want 1", *selfCalls)
	}
	if len(*proxyHosts) != 0 {
		t.Fatalf("expected no proxy calls, got %v", *proxyHosts)
	}
}

func TestRestartActionHandler_VlcProxiesToVlcServerHost(t *testing.T) {
	savedHost := c.Conf.VlcServerHost
	t.Cleanup(func() { c.Conf.VlcServerHost = savedHost })
	c.Conf.VlcServerHost = "vlc-server.example:8080"
	selfCalls, proxyHosts := withRestart(t)

	r := mux.NewRouter()
	r.Handle("/admin/restart/{service}", http.HandlerFunc(restartActionHandler)).Methods("POST")
	req := httptest.NewRequest(http.MethodPost, "/admin/restart/vlc-server", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if *selfCalls != 0 {
		t.Fatalf("restartSelf called %d times, want 0", *selfCalls)
	}
	if got, want := *proxyHosts, []string{"vlc-server.example:8080"}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("got proxy hosts %v, want %v", got, want)
	}
}

func TestRestartActionHandler_ObsRequiresAdminHost(t *testing.T) {
	savedHost := c.Conf.ObsServerHost
	t.Cleanup(func() { c.Conf.ObsServerHost = savedHost })
	c.Conf.ObsServerHost = "" // not configured
	withRestart(t)

	r := mux.NewRouter()
	r.Handle("/admin/restart/{service}", http.HandlerFunc(restartActionHandler)).Methods("POST")
	req := httptest.NewRequest(http.MethodPost, "/admin/restart/obs", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestRestartActionHandler_UnknownServiceIs400(t *testing.T) {
	withRestart(t)

	r := mux.NewRouter()
	r.Handle("/admin/restart/{service}", http.HandlerFunc(restartActionHandler)).Methods("POST")
	req := httptest.NewRequest(http.MethodPost, "/admin/restart/bogus", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAdminHandler_RendersReadyStatusAndLinks(t *testing.T) {
	defer SetTwitchConnected(false)
	SetTwitchConnected(true)
	withReauth(t, nil) // healthy tokens — no re-auth callout
	withNowPlaying(t, nowPlayingTrack{Artist: "Test Artist", Title: "Test Track"})

	// stand in for vlc-server: readiness ping + version endpoint
	vlc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health/ready":
			w.WriteHeader(http.StatusOK)
		case "/version":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"tag":"v9.9.9-vlc","sha":"deadbeefcafe"}`))
		default:
			t.Errorf("unexpected vlc request %q", r.URL.Path)
		}
	}))
	defer vlc.Close()

	// stand in for onscreens-server: same /health/ready + /version surface
	onscreens := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health/ready":
			w.WriteHeader(http.StatusOK)
		case "/version":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"tag":"v8.8.8-osc","sha":"feedfacecafe"}`))
		default:
			t.Errorf("unexpected onscreens request %q", r.URL.Path)
		}
	}))
	defer onscreens.Close()

	withConf(t, func() {
		c.Conf.VlcServerHost = strings.TrimPrefix(vlc.URL, "http://")
		c.Conf.OnscreensServerHost = strings.TrimPrefix(onscreens.URL, "http://")
		c.Conf.ChannelName = "adanalife_"
		c.Conf.BotUsername = "tripbot4000"
		c.Conf.ExternalURL = "https://tripbot.prod.whereisdana.today"
		c.Conf.Environment = "production"
	})
	withCurrentlyPlaying(t, video.Video{Slug: "wy_0042", State: "Wyoming"}, 3*time.Minute+12*time.Second)
	withChatterCount(t, 12)

	saved := versionTag
	defer func() { versionTag = saved }()
	SetVersion("v1.2.3")

	rec := httptest.NewRecorder()
	adminHandler(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<a href="https://twitch.tv/adanalife_">adanalife_</a>`,   // broadcaster profile
		`<a href="https://twitch.tv/tripbot4000">tripbot4000</a>`, // bot profile
		"in chat",                   // tripbot chat-connection status
		"healthy",                   // vlc status (onscreens row also asserts "healthy" via the version-link check below)
		`/CHANGELOG.md">v1.2.3</a>`, // tripbot version tag → changelog (ref is sha or master)
		`<a href="https://github.com/adanalife/tripbot/blob/deadbeefcafe/CHANGELOG.md">v9.9.9-vlc</a>`, // vlc version → changelog@sha
		`<a href="https://github.com/adanalife/tripbot/blob/feedfacecafe/CHANGELOG.md">v8.8.8-osc</a>`, // onscreens version → changelog@sha
		">vlc-server<",       // vlc row label
		">onscreens-server<", // onscreens row label
		"12 in chat",                          // chatter count
		`<code class="env">production</code>`, // env in monospace chip
		"now playing",                         // now-playing section shown when vlc healthy
		"wy_0042.MP4",                         // current video file
		"Wyoming",                             // current video state
		"3m12s",                               // clip progress
		`>obs</a>`,                            // one-word OBS link
		`>grafana</a>`,                        // one-word grafana link
		`>traefik</a>`,                        // one-word traefik link
		`>hubble</a>`,                         // one-word hubble link
		"https://obs.prod.whereisdana.today",  // derived OBS href
		grafanaURL,                            // grafana href
		traefikURL,                            // traefik href
		// Environment is "production" above → hubble link carries ?namespace=prod-1
		"https://hubble.prod.whereisdana.today/?namespace=prod-1",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}

func TestAdminHandler_DegradedAndVlcUnreachable(t *testing.T) {
	withNowPlaying(t, nowPlayingTrack{}) // no audio info — keeps assertion on "now playing" absence valid
	defer SetTwitchConnected(false)
	SetTwitchConnected(false)
	withReauth(t, nil)

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
	adminHandler(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "not in chat") {
		t.Errorf("body should report tripbot not in chat; got %q", body)
	}
	if !strings.Contains(body, "unreachable") {
		t.Errorf("body should report vlc unreachable; got %q", body)
	}
	if strings.Contains(body, "now playing") {
		t.Errorf("now-playing should be hidden when vlc is unhealthy; got %q", body)
	}
}

// withConf snapshots the config fields the admin handler reads, runs set to
// mutate them for the test, and restores them afterward.
func withConf(t *testing.T, set func()) {
	t.Helper()
	saved := struct{ vlc, onscreens, channel, bot, external, env string }{
		c.Conf.VlcServerHost, c.Conf.OnscreensServerHost, c.Conf.ChannelName, c.Conf.BotUsername, c.Conf.ExternalURL, c.Conf.Environment,
	}
	t.Cleanup(func() {
		c.Conf.VlcServerHost = saved.vlc
		c.Conf.OnscreensServerHost = saved.onscreens
		c.Conf.ChannelName = saved.channel
		c.Conf.BotUsername = saved.bot
		c.Conf.ExternalURL = saved.external
		c.Conf.Environment = saved.env
	})
	set()
}

// withCurrentlyPlaying swaps the currentlyPlaying / currentProgress seams so
// the admin handler sees a fixed video + progress without driving the player.
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

// withReauth swaps the accountsNeedingReauth seam so the admin handler sees a
// fixed re-auth list without depending on global in-memory token state.
func withReauth(t *testing.T, accounts []mytwitch.AccountReauth) {
	t.Helper()
	saved := accountsNeedingReauth
	accountsNeedingReauth = func() []mytwitch.AccountReauth { return accounts }
	t.Cleanup(func() { accountsNeedingReauth = saved })
}

// withNowPlaying swaps the SomaFM fetcher so the admin handler sees a fixed
// audio track (or empty for "no audio info") without hitting somafm.com from
// a test.
func withNowPlaying(t *testing.T, track nowPlayingTrack) {
	t.Helper()
	saved := nowPlayingFetcher
	nowPlayingFetcher = func(context.Context) nowPlayingTrack { return track }
	t.Cleanup(func() { nowPlayingFetcher = saved })
}

func TestAdminHandler_RendersReauthPrompt(t *testing.T) {
	defer SetTwitchConnected(false)
	SetTwitchConnected(false)
	withNowPlaying(t, nowPlayingTrack{})

	withConf(t, func() {
		c.Conf.VlcServerHost = "" // skip the vlc ping
		c.Conf.ChannelName = "adanalife_"
		c.Conf.BotUsername = "tripbot4000"
		c.Conf.ExternalURL = "https://tripbot.prod.whereisdana.today"
	})
	withReauth(t, []mytwitch.AccountReauth{
		{Account: "bot", LoginAs: "tripbot4000", Reason: "missing", InitURL: "https://tripbot.prod.whereisdana.today/auth/init?account=bot&login_as=tripbot4000"},
		{Account: "broadcaster", LoginAs: "adanalife_", Reason: "expired", InitURL: "https://tripbot.prod.whereisdana.today/auth/init?account=broadcaster&login_as=adanalife_"},
	})

	rec := httptest.NewRecorder()
	adminHandler(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	body := rec.Body.String()
	for _, want := range []string{
		"action needed: re-authenticate",
		// html/template escapes & as &amp; in attribute values (valid HTML).
		`href="https://tripbot.prod.whereisdana.today/auth/init?account=bot&amp;login_as=tripbot4000"`,
		"Sign in as tripbot4000",
		"(bot · missing)",
		`href="https://tripbot.prod.whereisdana.today/auth/init?account=broadcaster&amp;login_as=adanalife_"`,
		"Sign in as adanalife_",
		"(broadcaster · expired)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}

func TestAdminHandler_NoReauthPromptWhenHealthy(t *testing.T) {
	withNowPlaying(t, nowPlayingTrack{})
	defer SetTwitchConnected(false)
	SetTwitchConnected(true)

	withConf(t, func() { c.Conf.VlcServerHost = "" })
	withReauth(t, nil) // all tokens healthy

	rec := httptest.NewRecorder()
	adminHandler(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if strings.Contains(rec.Body.String(), "re-authenticate") {
		t.Errorf("re-auth prompt should be hidden when no account needs re-auth")
	}
}
