package server

import (
	"context"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"strings"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
	"github.com/adanalife/tripbot/pkg/video"
)

// startedAt marks process start so the admin panel can report uptime. Set at
// package load; close enough to process start for a human-readable "up Xh".
var startedAt = time.Now()

// healthClient is the short-timeout client used for the sibling-service status
// ping. 2s keeps a slow or hung vlc-server from stalling the panel render.
var healthClient = &http.Client{Timeout: 2 * time.Second}

// currentlyPlaying / currentProgress are overridable in tests; by default they
// read pkg/video's in-process state (refreshed by a 60s cron tick), so the
// admin panel costs nothing to show "now playing".
var (
	currentlyPlaying = video.CurrentlyPlaying
	currentProgress  = video.CurrentProgress
	// chatterCount is the in-memory count of users in chat, refreshed ~60s by
	// the UpdateSession cron. Overridable in tests.
	chatterCount = mytwitch.ChatterCount
	// accountsNeedingReauth reports which Twitch accounts have a missing/expired
	// token; the admin panel renders a re-auth prompt for each. Overridable in
	// tests. Reads in-memory token state — no DB or network call.
	accountsNeedingReauth = mytwitch.AccountsNeedingReauth
)

const (
	// grafanaURL points at the TripBot dashboards folder in Grafana Cloud.
	// Fixed — the org URL doesn't vary by environment.
	grafanaURL = "https://adanalife.grafana.net/dashboards/f/fflhs4m586io0e"
	// githubURL is the tripbot source repo (vlc-server + obs live here too).
	githubURL = "https://github.com/adanalife/tripbot"
	// traefikURL / hubbleURL are the cluster's platform dashboards. They live
	// in kube-system as a single prod-zone install shared across envs, so
	// (unlike OBS) they aren't derived per-environment.
	traefikURL = "https://traefik.prod.whereisdana.today"
	// hubbleBaseURL is the single prod-zone Hubble install on the mini-PC
	// cluster. gatherLinks appends ?namespace=<env's namespace> so the link
	// lands straight in that namespace's flow view instead of Hubble's "choose
	// a namespace" page — see hubbleNamespace.
	hubbleBaseURL = "https://hubble.prod.whereisdana.today/"
	// sentryURL is the org's issue list. gatherLinks appends ?environment=<env>
	// so the link lands pre-filtered to this env's issues — see sentryEnv.
	sentryURL = "https://a-dana-life.sentry.io/issues/"
)

// serviceStatus is one row in the admin panel's status table.
type serviceStatus struct {
	Name       string
	OK         bool
	Detail     string
	Version    string // optional build tag (e.g. vlc-server's, from /version)
	VersionURL string // changelog link (at the build's sha) for Version
	Uptime     string // human-readable "up Xh"; empty when the service is unreachable
}

// versionInfo is the subset of a service's /version JSON the page uses.
type versionInfo struct {
	Tag       string `json:"tag"`
	Sha       string `json:"sha"`
	StartedAt string `json:"started_at"` // RFC3339; used to derive uptime locally
}

// nowPlaying is the current-video summary shown when vlc-server is healthy.
type nowPlaying struct {
	File     string
	State    string
	Progress string
}

// navLink is one entry in the admin panel's links list.
type navLink struct {
	Label string
	URL   string
}

// adminData is the template payload.
type adminData struct {
	Channel  string // broadcaster Twitch username
	Bot      string // bot Twitch username
	Env      string
	Uptime   string
	Chatters int // users currently in chat
	Services []serviceStatus
	Now      *nowPlaying // nil when vlc is unhealthy or nothing is playing
	Links    []navLink
	Reauth   []mytwitch.AccountReauth // accounts whose token needs re-auth; empty when healthy
}

// adminHandler serves the human-facing root page on the tripbot Ingress: a
// lightweight status overview (tripbot's own readiness + a live vlc-server
// ping, each version linking to its changelog), the currently-playing video
// when vlc is up, the broadcaster/bot accounts, and links to the OBS / Grafana
// / Traefik / Hubble dashboards. Replaces the bare 404 that used to sit on "/".
func adminHandler(w http.ResponseWriter, r *http.Request) {
	vlcOK := c.Conf.VlcServerHost != "" &&
		pingHealthy(r.Context(), "http://"+c.Conf.VlcServerHost+"/health/ready")

	// vlc-server's build info — one extra in-cluster GET, only when it's up.
	var vlcVer versionInfo
	if vlcOK {
		vlcVer = fetchVersion(r.Context(), c.Conf.VlcServerHost)
	}

	data := adminData{
		Channel:  c.Conf.ChannelName,
		Bot:      c.Conf.BotUsername,
		Env:      c.Conf.Environment,
		Uptime:   time.Since(startedAt).Round(time.Second).String(),
		Chatters: chatterCount(),
		Services: gatherStatus(vlcOK, vlcVer, buildSHA()),
		Now:      currentVideo(vlcOK),
		Links:    gatherLinks(),
		Reauth:   accountsNeedingReauth(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := adminTmpl.Execute(w, data); err != nil {
		slog.ErrorContext(r.Context(), "couldn't render admin panel", "err", err)
	}
}

// gatherStatus reports tripbot's own readiness (in-memory, free) and folds in
// the already-computed vlc-server health. Each row carries its build tag in a
// version column linking to that build's changelog: tripbot's own tag (at sha,
// the running binary's VCS revision) and vlc-server's (from its /version). The
// vlc ping is best-effort: any failure (DNS, timeout, non-2xx) renders as not-OK
// rather than erroring the page.
func gatherStatus(vlcOK bool, vlcVer versionInfo, sha string) []serviceStatus {
	tripbot := serviceStatus{
		Name:       "tripbot",
		OK:         twitchConnected.Load(),
		Version:    versionTag,
		VersionURL: changelogURL(sha),
		Uptime:     uptimeSince(startedAt),
	}
	if tripbot.OK {
		tripbot.Detail = "in chat"
	} else {
		// The pod is up and serving this page — readiness no longer gates on
		// the Twitch connection — but the bot isn't connected to chat. The red
		// dot + this label is how that's surfaced; the re-auth links below
		// cover the common cause (a missing/expired token).
		tripbot.Detail = "not in chat"
	}

	vlc := serviceStatus{Name: "vlc-server", OK: vlcOK, Detail: "unreachable"}
	if vlcOK {
		vlc.Detail = "healthy"
		vlc.Version = vlcVer.Tag
		vlc.VersionURL = changelogURL(vlcVer.Sha) // same repo as tripbot
		if t, err := time.Parse(time.RFC3339, vlcVer.StartedAt); err == nil {
			vlc.Uptime = uptimeSince(t)
		}
	}

	return []serviceStatus{tripbot, vlc}
}

// uptimeSince formats a "since" duration as the short Go duration string
// rounded to the second — matches the meta-line uptime format.
func uptimeSince(t time.Time) string {
	return time.Since(t).Round(time.Second).String()
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

// gatherLinks builds the dashboard-link list: OBS's Ingress (derived from this
// bot's own EXTERNAL_URL by swapping the leading subdomain label), plus the
// Grafana / Traefik / Hubble platform dashboards. Twitch profiles are rendered
// in the accounts section and the changelog hangs off the version tags, so
// neither appears here. Entries whose URL can't be derived are dropped.
func gatherLinks() []navLink {
	links := []navLink{}
	if obs := siblingURL(c.Conf.ExternalURL, "obs"); obs != "" {
		links = append(links, navLink{Label: "obs", URL: obs})
	}
	links = append(links,
		navLink{Label: "grafana", URL: grafanaURL},
		navLink{Label: "traefik", URL: traefikURL},
		navLink{Label: "hubble", URL: hubbleBaseURL + "?namespace=" + hubbleNamespace()},
		navLink{Label: "sentry", URL: sentryURL + "?environment=" + sentryEnv()},
	)
	return links
}

// sentryEnv returns the environment tag Sentry events carry, so the admin
// panel's "sentry" link lands pre-filtered to this env's issues. Reads
// SENTRY_ENVIRONMENT (what the sentry-go SDK uses) so the link tag stays
// in sync with the events. Falls back to ENV for cases (tests, unset envs)
// where SENTRY_ENVIRONMENT isn't set.
func sentryEnv() string {
	if v := os.Getenv("SENTRY_ENVIRONMENT"); v != "" {
		return v
	}
	return c.Conf.Environment
}

// hubbleNamespace returns the Kubernetes namespace to deep-link the Hubble flow
// view at. Hubble is a single prod-zone install on the mini-PC cluster where
// stage-1 and prod-1 co-tenant — the only namespaces it shows — and ENV
// distinguishes them: production → prod-1, staging → stage-1. dev/local also
// report ENV=staging (so they resolve to stage-1), but they run on a separate
// cluster Hubble doesn't cover, so their link is moot regardless.
func hubbleNamespace() string {
	if c.Conf.IsProduction() {
		return "prod-1"
	}
	return "stage-1"
}

// changelogURL links to CHANGELOG.md as of the deployed commit, so it shows the
// changelog for exactly what's running. Falls back to the default branch when
// the sha is unknown (e.g. a build without embedded VCS info).
func changelogURL(sha string) string {
	ref := "master"
	if sha != "" {
		ref = sha
	}
	return githubURL + "/blob/" + ref + "/CHANGELOG.md"
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

var adminTmpl = template.Must(template.New("admin").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>tripbot — {{.Channel}}</title>
<!-- favicons referenced from the website (single owner of brand assets) — see
     vault general/logo.md; apple-touch-icon gives a home-screen icon on phones -->
<link rel="icon" type="image/png" sizes="32x32" href="https://www.dana.lol/assets/favicon-32x32.png">
<link rel="icon" type="image/png" sizes="16x16" href="https://www.dana.lol/assets/favicon-16x16.png">
<link rel="apple-touch-icon" sizes="180x180" href="https://www.dana.lol/assets/apple-touch-icon.png">
<style>
  :root { color-scheme: dark; --mono: ui-monospace,SFMono-Regular,Menlo,Consolas,monospace; }
  body { background:#0a0a0a; color:#eee; font:clamp(14px,0.5vw + 11px,18px)/1.65 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif; margin:0; display:flex; min-height:100vh; align-items:center; justify-content:center; }
  main { width:min(92vw,520px); padding:clamp(24px,4vw,48px); }
  /* logo is the monochrome black mark; invert to white on the dark bg */
  .logo { width:clamp(44px,5vw,60px); height:auto; filter:invert(1); opacity:.92; display:block; margin:0 0 16px; }
  .logo-link { display:inline-block; }
  .logo-link:hover .logo { opacity:1; }
  h1 { font-size:clamp(20px,1.2vw + 15px,28px); margin:0 0 4px; letter-spacing:.02em; }
  .env { font-family:var(--mono); background:#1a1a1a; border:1px solid #262626; color:#cdd; padding:2px 7px; border-radius:5px; font-size:.5em; font-weight:normal; letter-spacing:0; vertical-align:middle; }
  .meta { color:#888; margin:0 0 2px; font-size:.92em; }
  .accounts { color:#666; margin:0 0 24px; font-size:.85em; }
  h2 { font-size:.8em; text-transform:uppercase; letter-spacing:.08em; color:#888; margin:24px 0 8px; }
  ul { list-style:none; margin:0; padding:0; }
  .row { display:flex; align-items:center; gap:12px; padding:7px 0; border-bottom:1px solid #1c1c1c; }
  .row .name { flex:1; }
  .row .uptime { font-size:.85em; color:#666; }
  .row .ver { font-family:var(--mono); font-size:.85em; color:#777; }
  /* status hugs the far right and right-aligns so it forms a clean column,
     aligned whether or not the row carries a version */
  .row .status { color:#888; font-size:.92em; text-align:right; min-width:5.5em; }
  .dot { width:9px; height:9px; border-radius:50%; flex:0 0 auto; }
  .up { background:#3fb950; box-shadow:0 0 6px #3fb95080; }
  .down { background:#f85149; box-shadow:0 0 6px #f8514980; }
  .now { margin:0; padding:7px 0; }
  .now .file { font-family:var(--mono); word-break:break-all; }
  .now .state { color:#888; }
  a { color:#58a6ff; text-decoration:none; }
  a:hover { color:#9cf; }
  .links { display:flex; flex-wrap:wrap; gap:18px; margin-top:10px; }
  /* re-auth callout: only rendered when a token is missing/expired */
  .reauth { background:#3a1d00; border:1px solid #6b3b00; border-radius:8px; padding:14px 16px; margin:0 0 24px; }
  .reauth h2 { margin:0 0 8px; color:#ffb454; }
  .reauth p { margin:0 0 10px; color:#e8c89a; font-size:.9em; }
  .reauth .btns { display:flex; flex-wrap:wrap; gap:10px; }
  .reauth a.btn { display:inline-block; background:#ffb454; color:#1a1100; font-weight:600; padding:8px 14px; border-radius:6px; }
  .reauth a.btn:hover { background:#ffc578; color:#1a1100; }
  .reauth .why { font-family:var(--mono); font-size:.85em; opacity:.85; }
</style>
</head>
<body>
<main>
  <!-- A Dana Life mark, referenced from the website (the single owner of brand
       assets) rather than copied in — see vault general/logo.md. The anchor
       wraps the mark so clicking it refreshes the page. -->
  <a class="logo-link" href="/" title="refresh"><img class="logo" src="https://www.dana.lol/assets/logo.png" alt="A Dana Life" width="44" height="44"></a>
  <h1>tripbot <code class="env">{{.Env}}</code></h1>
  <p class="meta">up {{.Uptime}} · {{.Chatters}} in chat</p>
  <p class="accounts">broadcaster <a href="https://twitch.tv/{{.Channel}}">{{.Channel}}</a> · bot <a href="https://twitch.tv/{{.Bot}}">{{.Bot}}</a></p>

  {{if .Reauth}}
  <div class="reauth">
    <h2>action needed: re-authenticate</h2>
    <p>tripbot can't talk to Twitch until these accounts are re-authorized. Sign in as the named account on each — the flow re-prompts which account to use, so sign out of Twitch (or use a private window) if it grabs the wrong one.</p>
    <div class="btns">
      {{range .Reauth}}<a class="btn" href="{{.InitURL}}">Sign in as {{.LoginAs}} <span class="why">({{.Account}} · {{.Reason}})</span></a>{{end}}
    </div>
  </div>
  {{end}}

  <h2>status</h2>
  <ul>
    {{range .Services}}
    <li class="row">
      <span class="dot {{if .OK}}up{{else}}down{{end}}"></span>
      <span class="name">{{.Name}}</span>
      {{if .Uptime}}<span class="uptime">up {{.Uptime}}</span>{{end}}
      {{if .Version}}<span class="ver">{{if .VersionURL}}<a href="{{.VersionURL}}">{{.Version}}</a>{{else}}{{.Version}}{{end}}</span>{{end}}
      <span class="status">{{.Detail}}</span>
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

  <h2>dashboards</h2>
  <nav class="links">
    {{range .Links}}<a href="{{.URL}}">{{.Label}}</a>{{end}}
  </nav>
</main>
</body>
</html>`))
