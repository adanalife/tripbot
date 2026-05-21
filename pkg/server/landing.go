package server

import (
	"context"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
	"github.com/adanalife/tripbot/pkg/video"
)

// startedAt marks process start so the landing page can report uptime. Set at
// package load; close enough to process start for a human-readable "up Xh".
var startedAt = time.Now()

// healthClient is the short-timeout client used for the sibling-service status
// ping. 2s keeps a slow or hung vlc-server from stalling the landing render.
var healthClient = &http.Client{Timeout: 2 * time.Second}

// currentlyPlaying / currentProgress are overridable in tests; by default they
// read pkg/video's in-process state (refreshed by a 60s cron tick), so the
// landing page costs nothing to show "now playing".
var (
	currentlyPlaying = video.CurrentlyPlaying
	currentProgress  = video.CurrentProgress
	// chatterCount is the in-memory count of users in chat, refreshed ~60s by
	// the UpdateSession cron. Overridable in tests.
	chatterCount = mytwitch.ChatterCount
)

const (
	// grafanaURL points at the Grafana Cloud dashboards list (the TripBot
	// folder lives there). Fixed — the org URL doesn't vary by environment.
	grafanaURL = "https://adanalife.grafana.net/dashboards"
	// githubURL is the tripbot source repo (vlc-server + obs live here too).
	githubURL = "https://github.com/adanalife/tripbot"
)

// serviceStatus is one row in the landing page's status table.
type serviceStatus struct {
	Name       string
	OK         bool
	Detail     string
	Version    string // optional build tag (e.g. vlc-server's, from /version)
	VersionURL string // commit link for Version, when the sha is known
}

// versionInfo is the subset of a service's /version JSON the page uses.
type versionInfo struct {
	Tag string `json:"tag"`
	Sha string `json:"sha"`
}

// nowPlaying is the current-video summary shown when vlc-server is healthy.
type nowPlaying struct {
	File     string
	State    string
	Progress string
}

// navLink is one entry in the landing page's links list.
type navLink struct {
	Label string
	URL   string
}

// landingData is the template payload.
type landingData struct {
	Channel   string
	Env       string
	Uptime    string
	Version   string // tripbot's own build tag
	SHA       string // short git sha
	CommitURL string // link to the build's commit on GitHub (empty if no sha)
	Chatters  int    // users currently in chat
	Services  []serviceStatus
	Now       *nowPlaying // nil when vlc is unhealthy or nothing is playing
	Links     []navLink
}

// landingHandler serves the human-facing root page on the tripbot Ingress: a
// lightweight status overview (tripbot's own readiness + a live vlc-server
// ping), the currently-playing video when vlc is up, and links out to OBS,
// Grafana, and the Twitch channel. Replaces the bare 404 that used to sit on
// "/".
func landingHandler(w http.ResponseWriter, r *http.Request) {
	vlcOK := c.Conf.VlcServerHost != "" &&
		pingHealthy(r.Context(), "http://"+c.Conf.VlcServerHost+"/health/ready")

	// vlc-server's build info — one extra in-cluster GET, only when it's up.
	var vlcVer versionInfo
	if vlcOK {
		vlcVer = fetchVersion(r.Context(), c.Conf.VlcServerHost)
	}

	data := landingData{
		Channel:  c.Conf.ChannelName,
		Env:      c.Conf.Environment,
		Uptime:   time.Since(startedAt).Round(time.Second).String(),
		Version:  versionTag,
		Chatters: chatterCount(),
		Services: gatherStatus(vlcOK, vlcVer),
		Now:      currentVideo(vlcOK),
		Links:    gatherLinks(),
	}
	if sha := buildSHA(); sha != "" {
		data.SHA = sha[:min(7, len(sha))]
		data.CommitURL = commitURL(sha)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := landingTmpl.Execute(w, data); err != nil {
		slog.ErrorContext(r.Context(), "couldn't render landing page", "err", err)
	}
}

// gatherStatus reports tripbot's own readiness (in-memory, free) and folds in
// the already-computed vlc-server health (with its build tag when reachable).
// The vlc ping is best-effort: any failure (DNS, timeout, non-2xx) renders as
// not-OK rather than erroring the page.
func gatherStatus(vlcOK bool, vlcVer versionInfo) []serviceStatus {
	tripbot := serviceStatus{Name: "tripbot", OK: ready.Load()}
	if tripbot.OK {
		tripbot.Detail = "ready"
	} else {
		tripbot.Detail = "degraded (awaiting Twitch)"
	}

	vlc := serviceStatus{Name: "vlc-server", OK: vlcOK, Detail: "unreachable"}
	if vlcOK {
		vlc.Detail = "healthy"
		vlc.Version = vlcVer.Tag
		vlc.VersionURL = commitURL(vlcVer.Sha) // same repo as tripbot
	}

	return []serviceStatus{tripbot, vlc}
}

// commitURL links a git sha to its commit on GitHub, or "" when sha is empty.
func commitURL(sha string) string {
	if sha == "" {
		return ""
	}
	return githubURL + "/commit/" + sha
}

// currentVideo summarizes the currently-playing video for the page, but only
// when vlc is healthy (a stale value while vlc is down would be misleading).
// Reads pkg/video's in-process value — no extra call. Returns nil when nothing
// is playing yet (empty slug).
func currentVideo(vlcOK bool) *nowPlaying {
	if !vlcOK {
		return nil
	}
	v := currentlyPlaying()
	if v.Slug == "" {
		return nil
	}
	return &nowPlaying{
		File:     v.File(),
		State:    v.State,
		Progress: currentProgress().Round(time.Second).String(),
	}
}

// buildSHA returns the binary's embedded VCS revision (via Go's -buildvcs), or
// "" for builds without it (e.g. `go test`). Same source versionHandler uses.
func buildSHA() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				return s.Value
			}
		}
	}
	return ""
}

// fetchVersion GETs a sibling service's /version endpoint and returns its build
// tag + sha, or a zero versionInfo on any error. Uses the same short-timeout
// client as the health ping.
func fetchVersion(ctx context.Context, host string) versionInfo {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+host+"/version", nil)
	if err != nil {
		return versionInfo{}
	}
	resp, err := healthClient.Do(req)
	if err != nil {
		return versionInfo{}
	}
	defer resp.Body.Close()
	var v versionInfo
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return versionInfo{}
	}
	return v
}

// pingHealthy GETs a readiness URL and reports whether it answered 2xx within
// the client timeout.
func pingHealthy(ctx context.Context, rawURL string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false
	}
	resp, err := healthClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// gatherLinks builds the external-link list: OBS's Ingress (derived from this
// bot's own EXTERNAL_URL by swapping the leading subdomain label), the Grafana
// dashboards, the Twitch channel, and the GitHub source repo. Entries whose
// URL can't be derived are dropped rather than rendered broken.
func gatherLinks() []navLink {
	links := []navLink{}
	if obs := siblingURL(c.Conf.ExternalURL, "obs"); obs != "" {
		links = append(links, navLink{Label: "OBS (noVNC)", URL: obs})
	}
	links = append(links, navLink{Label: "Grafana dashboards", URL: grafanaURL})
	if c.Conf.ChannelName != "" {
		links = append(links, navLink{Label: "Twitch channel", URL: "https://twitch.tv/" + c.Conf.ChannelName})
	}
	links = append(links, navLink{Label: "GitHub (source)", URL: githubURL})
	return links
}

// siblingURL rewrites externalURL's leading hostname label to service, e.g.
// https://tripbot.prod.whereisdana.today -> https://obs.prod.whereisdana.today.
// Returns "" when externalURL isn't a multi-label FQDN (e.g. localhost), since
// there's no sibling Ingress to point at in that case.
func siblingURL(externalURL, service string) string {
	u, err := url.Parse(externalURL)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	_, rest, found := strings.Cut(u.Hostname(), ".")
	if !found {
		return ""
	}
	u.Host = service + "." + rest
	return u.String()
}

var landingTmpl = template.Must(template.New("landing").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>tripbot — {{.Channel}}</title>
<style>
  :root { color-scheme: dark; }
  body { background:#0a0a0a; color:#eee; font:14px/1.6 -apple-system,BlinkMacSystemFont,"Segoe UI",monospace; margin:0; display:flex; min-height:100vh; align-items:center; justify-content:center; }
  main { width:min(92vw,420px); padding:32px; }
  h1 { font-size:20px; margin:0 0 2px; letter-spacing:.02em; }
  .ver { color:#666; margin:0 0 10px; font-size:12px; }
  .meta { color:#888; margin:0 0 24px; font-size:13px; }
  h2 { font-size:12px; text-transform:uppercase; letter-spacing:.08em; color:#888; margin:24px 0 8px; }
  ul { list-style:none; margin:0; padding:0; }
  .svc { display:flex; align-items:center; gap:10px; padding:6px 0; border-bottom:1px solid #1c1c1c; }
  .svc .name { flex:1; }
  .svc .detail { color:#888; font-size:13px; }
  .dot { width:9px; height:9px; border-radius:50%; flex:0 0 auto; }
  .up { background:#3fb950; box-shadow:0 0 6px #3fb95080; }
  .down { background:#f85149; box-shadow:0 0 6px #f8514980; }
  .now { margin:0; padding:6px 0; }
  .now .file { word-break:break-all; }
  .now .state { color:#888; }
  a { color:#58a6ff; text-decoration:none; }
  a:hover { color:#9cf; }
  .links a { display:block; padding:6px 0; border-bottom:1px solid #1c1c1c; }
</style>
</head>
<body>
<main>
  <h1>tripbot</h1>
  {{if .Version}}<p class="ver">{{.Version}}{{if .SHA}} · <a href="{{.CommitURL}}">{{.SHA}}</a>{{end}}</p>{{end}}
  <p class="meta">channel <strong><a href="https://twitch.tv/{{.Channel}}">{{.Channel}}</a></strong> · env {{.Env}} · up {{.Uptime}} · {{.Chatters}} in chat</p>

  <h2>status</h2>
  <ul>
    {{range .Services}}
    <li class="svc">
      <span class="dot {{if .OK}}up{{else}}down{{end}}"></span>
      <span class="name">{{.Name}}</span>
      <span class="detail">{{.Detail}}{{if .Version}} · {{if .VersionURL}}<a href="{{.VersionURL}}">{{.Version}}</a>{{else}}{{.Version}}{{end}}{{end}}</span>
    </li>
    {{end}}
  </ul>

  {{with .Now}}
  <h2>now playing</h2>
  <p class="now">
    <span class="file">{{.File}}</span>
    {{if .State}}<span class="state">· {{.State}}</span>{{end}}
    {{if .Progress}}<span class="state">· {{.Progress}}</span>{{end}}
  </p>
  {{end}}

  <h2>links</h2>
  <ul class="links">
    {{range .Links}}
    <li><a href="{{.URL}}">{{.Label}} →</a></li>
    {{end}}
  </ul>
</main>
</body>
</html>`))
