package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
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

func TestChangelogURL(t *testing.T) {
	if got, want := changelogURL("deadbeef"), githubURL+"/blob/deadbeef/CHANGELOG.md"; got != want {
		t.Errorf("changelogURL(sha) = %q, want %q", got, want)
	}
	if got, want := changelogURL(""), githubURL+"/blob/master/CHANGELOG.md"; got != want {
		t.Errorf("changelogURL(\"\") = %q, want %q", got, want)
	}
}

func TestLandingHandler_RendersReadyStatusAndLinks(t *testing.T) {
	defer SetTwitchConnected(false)
	SetTwitchConnected(true)
	withReauth(t, nil) // healthy tokens — no re-auth callout

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

	withConf(t, func() {
		c.Conf.VlcServerHost = strings.TrimPrefix(vlc.URL, "http://")
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
	landingHandler(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<a href="https://twitch.tv/adanalife_">adanalife_</a>`,   // broadcaster profile
		`<a href="https://twitch.tv/tripbot4000">tripbot4000</a>`, // bot profile
		"in chat",                   // tripbot chat-connection status
		"healthy",                   // vlc status
		`/CHANGELOG.md">v1.2.3</a>`, // tripbot version tag → changelog (ref is sha or master)
		`<a href="https://github.com/adanalife/tripbot/blob/deadbeefcafe/CHANGELOG.md">v9.9.9-vlc</a>`, // vlc version → changelog@sha
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
		hubbleURL,                             // hubble href
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}

func TestLandingHandler_DegradedAndVlcUnreachable(t *testing.T) {
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
	landingHandler(rec, httptest.NewRequest(http.MethodGet, "/", nil))

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

// withConf snapshots the config fields the landing handler reads, runs set to
// mutate them for the test, and restores them afterward.
func withConf(t *testing.T, set func()) {
	t.Helper()
	saved := struct{ vlc, channel, bot, external, env string }{
		c.Conf.VlcServerHost, c.Conf.ChannelName, c.Conf.BotUsername, c.Conf.ExternalURL, c.Conf.Environment,
	}
	t.Cleanup(func() {
		c.Conf.VlcServerHost = saved.vlc
		c.Conf.ChannelName = saved.channel
		c.Conf.BotUsername = saved.bot
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

// withReauth swaps the accountsNeedingReauth seam so the landing handler sees a
// fixed re-auth list without depending on global in-memory token state.
func withReauth(t *testing.T, accounts []mytwitch.AccountReauth) {
	t.Helper()
	saved := accountsNeedingReauth
	accountsNeedingReauth = func() []mytwitch.AccountReauth { return accounts }
	t.Cleanup(func() { accountsNeedingReauth = saved })
}

func TestLandingHandler_RendersReauthPrompt(t *testing.T) {
	defer SetTwitchConnected(false)
	SetTwitchConnected(false)

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
	landingHandler(rec, httptest.NewRequest(http.MethodGet, "/", nil))

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

func TestLandingHandler_NoReauthPromptWhenHealthy(t *testing.T) {
	defer SetTwitchConnected(false)
	SetTwitchConnected(true)

	withConf(t, func() { c.Conf.VlcServerHost = "" })
	withReauth(t, nil) // all tokens healthy

	rec := httptest.NewRecorder()
	landingHandler(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if strings.Contains(rec.Body.String(), "re-authenticate") {
		t.Errorf("re-auth prompt should be hidden when no account needs re-auth")
	}
}
