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
	"github.com/adanalife/tripbot/pkg/feature"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
	"github.com/adanalife/tripbot/pkg/video"
	"github.com/gorilla/mux"
)

func TestPanelHost(t *testing.T) {
	cases := []struct {
		host, want string
	}{
		{"tripbot.prod.whereisdana.today", "tripbot.prod.whereisdana.today"},
		{"tripbot.prod.whereisdana.today:8080", "tripbot.prod.whereisdana.today"},
		{"localhost:8080", "localhost"},
		{"adanalife-minipc.tail020deb.ts.net", "adanalife-minipc.tail020deb.ts.net"},
		{"", ""},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = tc.host
		if got := panelHost(req); got != tc.want {
			t.Errorf("panelHost(Host=%q) = %q, want %q", tc.host, got, tc.want)
		}
	}
}

func TestTailnetServiceURL(t *testing.T) {
	saved := c.Conf.Environment
	t.Cleanup(func() { c.Conf.Environment = saved })
	cases := []struct {
		env, service, want string
	}{
		{"production", "obs", "https://obs-prod.tail020deb.ts.net"},
		{"production", "vlc-server", "https://vlc-server-prod.tail020deb.ts.net"},
		{"staging", "obs", "https://obs-stage.tail020deb.ts.net"},
		{"development", "obs", ""}, // not served by the operator
		{"testing", "obs", ""},
	}
	for _, tc := range cases {
		c.Conf.Environment = tc.env
		if got := tailnetServiceURL(tc.service); got != tc.want {
			t.Errorf("tailnetServiceURL(%q) with ENV=%q = %q, want %q", tc.service, tc.env, got, tc.want)
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

func TestEnvColorClass(t *testing.T) {
	cases := map[string]string{
		"production":  "env-prod",
		"staging":     "env-stage",
		"development": "env-dev",
		"testing":     "", // neutral chip
		"":            "", // unset env falls through to neutral
		"weird":       "", // unknown env falls through to neutral
	}
	for env, want := range cases {
		if got := envColorClass(env); got != want {
			t.Errorf("envColorClass(%q) = %q, want %q", env, got, want)
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

func TestObsStreamActionHandler_StartSwapsInStopControl(t *testing.T) {
	started, _ := withObsStream(t, nil, nil)

	r := mux.NewRouter()
	r.Handle("/admin/obs/stream/{action}", http.HandlerFunc(obsStreamActionHandler)).Methods("POST")

	req := httptest.NewRequest(http.MethodPost, "/admin/obs/stream/start", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	// A successful start flips the widget to offer "stop".
	if !strings.Contains(body, `id="stream-control"`) {
		t.Errorf("response should be the stream-control fragment; got %q", body)
	}
	if !strings.Contains(body, `hx-post="/admin/obs/stream/stop"`) {
		t.Errorf("swapped-in widget should now offer stop; got %q", body)
	}
	// HX-Trigger tells the page the stream is now active (opens the preview).
	if got := rec.Header().Get("HX-Trigger"); !strings.Contains(got, `"active":true`) {
		t.Errorf("HX-Trigger = %q, want stream-changed active:true", got)
	}
	if *started != 1 {
		t.Fatalf("obsStartStream called %d times, want 1", *started)
	}
}

func TestObsStreamActionHandler_StopSwapsInStartControl(t *testing.T) {
	_, stopped := withObsStream(t, nil, nil)

	r := mux.NewRouter()
	r.Handle("/admin/obs/stream/{action}", http.HandlerFunc(obsStreamActionHandler)).Methods("POST")

	req := httptest.NewRequest(http.MethodPost, "/admin/obs/stream/stop", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `hx-post="/admin/obs/stream/start"`) {
		t.Errorf("swapped-in widget should now offer start; got %q", body)
	}
	if got := rec.Header().Get("HX-Trigger"); !strings.Contains(got, `"active":false`) {
		t.Errorf("HX-Trigger = %q, want stream-changed active:false", got)
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

func TestObsStreamActionHandler_ErrorLeavesStateUnchanged(t *testing.T) {
	// State is the source of truth — a failed toggle re-renders the previous
	// state, so the swapped-in widget simply doesn't flip. Surfacing the error in
	// flash UI isn't worth the complexity for a tailnet-only solo-operator panel;
	// the periodic refresh reconciles actual OBS state.
	started, _ := withObsStream(t, errFakeOBS, nil)

	r := mux.NewRouter()
	r.Handle("/admin/obs/stream/{action}", http.HandlerFunc(obsStreamActionHandler)).Methods("POST")

	req := httptest.NewRequest(http.MethodPost, "/admin/obs/stream/start", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	// Start failed → widget still offers start (didn't flip to stop).
	body := rec.Body.String()
	if !strings.Contains(body, `hx-post="/admin/obs/stream/start"`) {
		t.Errorf("failed start should leave the widget offering start; got %q", body)
	}
	if got := rec.Header().Get("HX-Trigger"); !strings.Contains(got, `"active":false`) {
		t.Errorf("HX-Trigger = %q, want stream-changed active:false", got)
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

	if rec.Code != http.StatusNoContent {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusNoContent)
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
	req := httptest.NewRequest(http.MethodPost, "/admin/restart/vlc", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusNoContent)
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

func TestRefreshHandler_RendersOOBStatusAndStreamControl(t *testing.T) {
	srv := New()
	srv.SetTwitchConnected(true)

	// stream active → the OOB widget should offer "stop"
	savedStatus := obsStreamStatus
	obsStreamStatus = func(context.Context) (bool, error) { return true, nil }
	t.Cleanup(func() { obsStreamStatus = savedStatus })

	// ObsServerHost isn't covered by withConf — save/restore it here.
	savedObs := c.Conf.ObsServerHost
	t.Cleanup(func() { c.Conf.ObsServerHost = savedObs })

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

	withConf(t, func() {
		c.Conf.VlcServerHost = strings.TrimPrefix(vlc.URL, "http://")
		c.Conf.OnscreensServerHost = "" // skip the onscreens ping
	})
	c.Conf.ObsServerHost = "" // skip the obs sibling ping

	rec := httptest.NewRecorder()
	srv.refreshHandler(rec, httptest.NewRequest(http.MethodGet, "/admin/refresh", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<ul id="status-list" hx-swap-oob="true">`, // status rows OOB target
		"in chat",                          // tripbot connected
		">vlc<",                            // vlc service row
		`hx-post="/admin/restart/vlc"`,     // restart button rendered in the rows
		`v9.9.9-vlc`,                       // vlc version pulled through
		`id="stream-control"`,              // stream widget present
		`hx-post="/admin/obs/stream/stop"`, // stream active → offers stop
	} {
		if !strings.Contains(body, want) {
			t.Errorf("refresh body missing %q", want)
		}
	}
	// The stream widget must be OOB-tagged so it swaps without a target attr.
	if !strings.Contains(body, `id="stream-control" class="stream-control" hx-swap-oob="true"`) {
		t.Errorf("stream-control should be OOB-tagged; got %q", body)
	}
}

func TestAdminHandler_RendersReadyStatusAndLinks(t *testing.T) {
	srv := New()
	srv.SetTwitchConnected(true)
	withReauth(t, nil) // healthy tokens — no re-auth callout
	withAuthStatuses(t, []mytwitch.AccountTokenStatus{
		{Account: "bot", LoginAs: "tripbot4000", ExpiresAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)},
	})
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

	srv.SetVersion("v1.2.3")

	rec := httptest.NewRecorder()
	srv.adminHandler(rec, httptest.NewRequest(http.MethodGet, "/", nil))

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
		">vlc<",                                  // vlc row label (shortened)
		">onscreens<",                            // onscreens row label (shortened)
		`<span class="chatters-count">12</span>`, // initial chatter count (server-rendered, unflashed)
		`id="chatters" sse-swap="viewers" hx-swap="innerHTML"`, // live count target wired for SSE updates
		`<code class="env env-prod">production</code>`,         // env in monospace chip, prod-coloured
		`<title>tripbot — adanalife_ (production)</title>`,     // env rendered in <title> for tab disambiguation
		"now playing",                                    // now-playing section shown when vlc healthy
		`id="now-line" sse-swap="video"`,                 // now-playing line wired for live video swaps
		`id="auth-card" sse-swap="auth"`,                 // live token-expiry card wired for SSE
		`class="auth-expires" data-expires="4070908800"`, // bot expiry (2099-01-01) for the JS countdown
		`id="reauth-card" sse-swap="reauth"`,             // reauth callout container wired for live appear/clear
		"wy_0042.MP4",                                    // current video file
		"Wyoming",                                        // current video state
		`class="now-elapsed" data-since=`,                // elapsed span the JS ticker counts up
		"3m12s",                                          // clip progress (initial server render)
		`>obs</a>`,                                       // one-word OBS link
		`>grafana</a>`,                                   // one-word grafana link
		`>traefik</a>`,                                   // one-word traefik link
		`>hubble</a>`,                                    // one-word hubble link
		"https://obs-prod.tail020deb.ts.net",             // tailnet OBS href
		grafanaURL,                                       // grafana href
		traefikURL,                                       // traefik href
		// Environment is "production" above → hubble link carries ?namespace=prod-1
		"https://hubble-prod.tail020deb.ts.net/?namespace=prod-1",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}

func TestAdminHandler_DegradedAndVlcUnreachable(t *testing.T) {
	withNowPlaying(t, nowPlayingTrack{}) // no audio info — keeps assertion on "now playing" absence valid
	srv := New()
	srv.SetTwitchConnected(false)
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
	srv.adminHandler(rec, httptest.NewRequest(http.MethodGet, "/", nil))

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

// withAuthStatuses swaps the authStatuses seam so the admin handler sees a
// fixed per-identity token state (for the live expiry-countdown card) without
// depending on global in-memory token state.
func withAuthStatuses(t *testing.T, statuses []mytwitch.AccountTokenStatus) {
	t.Helper()
	saved := authStatuses
	authStatuses = func() []mytwitch.AccountTokenStatus { return statuses }
	t.Cleanup(func() { authStatuses = saved })
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
	srv := New()
	srv.SetTwitchConnected(false)
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
	srv.adminHandler(rec, httptest.NewRequest(http.MethodGet, "/", nil))

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
	srv := New()
	srv.SetTwitchConnected(true)

	withConf(t, func() { c.Conf.VlcServerHost = "" })
	withReauth(t, nil) // all tokens healthy
	withAuthStatuses(t, []mytwitch.AccountTokenStatus{
		{Account: "bot", LoginAs: "tripbot4000", ExpiresAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)},
	})

	rec := httptest.NewRecorder()
	srv.adminHandler(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if strings.Contains(rec.Body.String(), "re-authenticate") {
		t.Errorf("re-auth prompt should be hidden when no account needs re-auth")
	}
}

func withFlags(t *testing.T, srv *Server, flags map[string]feature.Flag) {
	t.Helper()
	srv.SetFlagClient(feature.NewInMemoryClient(flags))
}

func TestAdminHandler_RendersFeatureFlagsWhenLoaded(t *testing.T) {
	srv := New()
	srv.SetTwitchConnected(true)
	withNowPlaying(t, nowPlayingTrack{})
	withReauth(t, nil)
	withConf(t, func() { c.Conf.VlcServerHost = "" })
	withFlags(t, srv, map[string]feature.Flag{
		"discord.bot_enabled": {
			Key:               "discord.bot_enabled",
			Description:       "Gates pkg/discord startup.",
			Enabled:           true,
			TargetRemovalDate: time.Date(2026, 11, 28, 0, 0, 0, 0, time.UTC),
		},
		"chatbot.experimental": {
			Key:               "chatbot.experimental",
			Description:       "Experimental chatbot path.",
			Enabled:           false,
			TargetRemovalDate: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		},
	})

	rec := httptest.NewRecorder()
	srv.adminHandler(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	body := rec.Body.String()
	for _, want := range []string{
		"feature flags",                     // disclosure label
		"<code>chatbot.experimental</code>", // monospace key
		"<code>discord.bot_enabled</code>",
		"Gates pkg/discord startup.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
	// Target-removal date is intentionally omitted from the panel render — it's
	// metadata for the future audit job / admin CRUD UI, not for the read-only
	// status surface — so it should never appear.
	if strings.Contains(body, "remove by") || strings.Contains(body, "2026-11-28") {
		t.Errorf("panel should not render target_removal_date")
	}
	// Sort order: keys ascend, so chatbot.experimental comes before discord.bot_enabled.
	if i, j := strings.Index(body, "chatbot.experimental"), strings.Index(body, "discord.bot_enabled"); i < 0 || j < 0 || i > j {
		t.Errorf("expected chatbot.experimental row before discord.bot_enabled; got positions %d, %d", i, j)
	}
}

func TestAdminHandler_HidesFeatureFlagsSectionWhenEmpty(t *testing.T) {
	srv := New()
	srv.SetTwitchConnected(true)
	withNowPlaying(t, nowPlayingTrack{})
	withReauth(t, nil)
	withConf(t, func() { c.Conf.VlcServerHost = "" })
	withFlags(t, srv, nil) // no flags loaded

	rec := httptest.NewRecorder()
	srv.adminHandler(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if strings.Contains(rec.Body.String(), "feature flags") {
		t.Errorf("feature flags section should be hidden when no flags are loaded")
	}
}

// withChatHistory seeds srv's hub with a fresh ring containing the given lines.
func withChatHistory(t *testing.T, srv *Server, lines []ChatLine) {
	t.Helper()
	h := NewHub()
	for _, l := range lines {
		h.appendChat(l)
	}
	srv.hub = h
}

func TestAdminHandler_RendersChatHistoryAndSSEWiring(t *testing.T) {
	srv := New()
	srv.SetTwitchConnected(true)
	withNowPlaying(t, nowPlayingTrack{})
	withReauth(t, nil)
	withConf(t, func() { c.Conf.VlcServerHost = "" })
	withChatHistory(t, srv, []ChatLine{
		{Username: "alice", Text: "hello", At: time.Date(2026, 5, 29, 13, 5, 0, 0, time.UTC)},
		{Username: "bob", Text: "<b>hi</b>", At: time.Date(2026, 5, 29, 13, 6, 0, 0, time.UTC)},
	})

	rec := httptest.NewRecorder()
	srv.adminHandler(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	body := rec.Body.String()
	for _, want := range []string{
		`sse-connect="/admin/events"`, // SSE wiring present
		`id="chat-log"`,               // live chat pane
		`/static/htmx.min.js`,         // vendored frontend loaded
		"alice", "hello",              // seeded history rendered
		"&lt;b&gt;hi&lt;/b&gt;", // chat text HTML-escaped
		`<time class="ct-ts"`,   // per-line timestamp rendered
		"13:05",                 // server-side UTC fallback (JS localizes)
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}

func TestAdminHandler_ChatEmptyPlaceholder(t *testing.T) {
	srv := New()
	srv.SetTwitchConnected(true)
	withNowPlaying(t, nowPlayingTrack{})
	withReauth(t, nil)
	withConf(t, func() { c.Conf.VlcServerHost = "" })
	withChatHistory(t, srv, nil) // empty ring

	rec := httptest.NewRecorder()
	srv.adminHandler(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if !strings.Contains(rec.Body.String(), "waiting for chat") {
		t.Errorf("expected empty-chat placeholder when no history")
	}
}
