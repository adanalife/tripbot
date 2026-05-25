package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"strings"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/httpmw"
	"github.com/adanalife/tripbot/pkg/obs"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
	"github.com/adanalife/tripbot/pkg/video"
	"github.com/gorilla/mux"
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
	// obsStreamStatus reports whether OBS is currently streaming. Function-seam
	// so handler tests can stub without opening a real OBS WebSocket. Default
	// hits OBS via pkg/obs (fresh connection per call — see obs.GetStreamStatus).
	obsStreamStatus = obs.GetStreamStatus
	// obsStartStream / obsStopStream send the toggle commands to OBS. Function
	// seams for the same reason. Default to pkg/obs.
	obsStartStream = obs.StartStream
	obsStopStream  = obs.StopStream
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

// streamControl drives the OBS stream toggle widget. Reachable=false hides
// the action button (we don't want to offer "Start" when OBS is unreachable
// — the click would just fail). When Reachable=true, Active selects the
// button label/style: "Stop stream" (red) when active, "Start stream"
// (green) when idle.
type streamControl struct {
	Reachable bool
	Active    bool
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
	Stream   streamControl
	Links    []navLink
	Reauth   []mytwitch.AccountReauth // accounts whose token needs re-auth; empty when healthy
}

// adminHandler serves the human-facing root page on the tripbot Ingress: a
// lightweight status overview (tripbot's own readiness + live sibling-service
// pings, each version linking to its changelog), the currently-playing video
// when vlc is up, the broadcaster/bot accounts, and links to the OBS / Grafana
// / Traefik / Hubble dashboards. Replaces the bare 404 that used to sit on "/".
func adminHandler(w http.ResponseWriter, r *http.Request) {
	vlc := siblingStatus(r.Context(), "vlc-server", c.Conf.VlcServerHost)
	onscreens := siblingStatus(r.Context(), "onscreens-server", c.Conf.OnscreensServerHost)
	obs := siblingStatus(r.Context(), "obs", c.Conf.ObsServerHost)

	data := adminData{
		Channel:  c.Conf.ChannelName,
		Bot:      c.Conf.BotUsername,
		Env:      c.Conf.Environment,
		Uptime:   time.Since(startedAt).Round(time.Second).String(),
		Chatters: chatterCount(),
		Services: gatherStatus(buildSHA(), vlc, onscreens, obs),
		Now:      currentVideo(vlc.OK),
		Stream:   gatherStream(r.Context()),
		Links:    gatherLinks(),
		Reauth:   accountsNeedingReauth(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := adminTmpl.Execute(w, data); err != nil {
		slog.ErrorContext(r.Context(), "couldn't render admin panel", "err", err)
	}
}

// gatherStatus reports tripbot's own readiness (in-memory, free) and folds in
// the already-probed sibling-service rows. Each row carries its build tag in a
// version column linking to that build's changelog at the deployed sha.
func gatherStatus(sha string, siblings ...serviceStatus) []serviceStatus {
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

	return append([]serviceStatus{tripbot}, siblings...)
}

// siblingStatus probes a sibling HTTP service (vlc-server, onscreens-server)
// and returns its row for the status table. Best-effort: an empty host or any
// readiness failure (DNS, timeout, non-2xx) renders as unreachable rather than
// erroring the page. When the probe succeeds, also fetches /version to fill in
// the build tag (linked to its changelog at the deployed sha) and uptime.
func siblingStatus(ctx context.Context, name, host string) serviceStatus {
	s := serviceStatus{Name: name, Detail: "unreachable"}
	if host == "" {
		return s
	}
	if !pingHealthy(ctx, "http://"+host+"/health/ready") {
		return s
	}
	s.OK = true
	s.Detail = "healthy"
	ver := fetchVersion(ctx, host)
	s.Version = ver.Tag
	s.VersionURL = changelogURL(ver.Sha) // same repo as tripbot
	if t, err := time.Parse(time.RFC3339, ver.StartedAt); err == nil {
		s.Uptime = uptimeSince(t)
	}
	return s
}

// uptimeSince formats a "since" duration as the short Go duration string
// rounded to the second — matches the meta-line uptime format.
func uptimeSince(t time.Time) string {
	return time.Since(t).Round(time.Second).String()
}

// gatherStream asks OBS for the current streaming state. A reachability
// failure (OBS down, password wrong) returns Reachable=false so the panel
// hides the toggle button rather than offering an action that would just
// fail. The OBS WebSocket connection is opened + closed inside obs.GetStreamStatus.
func gatherStream(ctx context.Context) streamControl {
	active, err := obsStreamStatus(ctx)
	if err != nil {
		// Reachability failure is a normal "OBS not running" condition on
		// dev clusters — log at debug, render as unreachable, move on.
		slog.DebugContext(ctx, "obs stream status unavailable", "err", err)
		return streamControl{Reachable: false}
	}
	return streamControl{Reachable: true, Active: active}
}

// restartHosts maps the {service} path variable to the host:port of that
// service's admin-shutdown endpoint. "tripbot" is the special case: routing
// to localhost would hit the SAME server, so we call httpmw.ShutdownHandler's
// signal seam directly (in-process) — no HTTP hop. Function-pointer seams
// for the proxy + in-process paths so tests don't open real sockets.
var (
	restartProxyShutdown = func(ctx context.Context, host string) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+host+"/admin/shutdown", nil)
		if err != nil {
			return err
		}
		resp, err := healthClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("shutdown returned %d", resp.StatusCode)
		}
		return nil
	}
	// restartSelf SIGTERMs tripbot itself by reusing httpmw's shared signal
	// seam. Same path the local /admin/shutdown handler uses — the only
	// difference is the trigger came from a button on the same page.
	restartSelf = func() error {
		go func() {
			time.Sleep(httpmw.ShutdownDelay)
			if err := httpmw.ShutdownSignal(); err != nil {
				slog.Error("admin restart-self signal failed", "err", err)
			}
		}()
		return nil
	}
)

// restartActionHandler handles POST /admin/restart/{service}. For tripbot it
// triggers an in-process self-shutdown (same shape as /admin/shutdown — the
// existing signal handler chain runs). For sibling services it proxies to
// their /admin/shutdown endpoint. Always 303-redirects to "/" so the panel
// reflects the new state on reload (tripbot's self-restart redirect resolves
// once the new pod is serving; until then the browser sees a brief gap).
func restartActionHandler(w http.ResponseWriter, r *http.Request) {
	service := mux.Vars(r)["service"]
	var err error
	switch service {
	case "tripbot":
		err = restartSelf()
	case "vlc-server":
		err = restartProxyShutdown(r.Context(), c.Conf.VlcServerHost)
	case "onscreens-server":
		err = restartProxyShutdown(r.Context(), c.Conf.OnscreensServerHost)
	case "obs":
		if c.Conf.ObsServerHost == "" {
			http.Error(w, "OBS_SERVER_HOST not configured", http.StatusBadRequest)
			return
		}
		err = restartProxyShutdown(r.Context(), c.Conf.ObsServerHost)
	default:
		http.Error(w, "unknown service", http.StatusBadRequest)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "admin restart failed", "service", service, "err", err)
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// obsStreamActionHandler handles POST /admin/obs/stream/{action} (action =
// "start" or "stop") from the admin panel's toggle form. Calls into pkg/obs
// to flip OBS's streaming state, then 303-redirects to "/" so the panel
// reflects the new state on reload. Errors log + still redirect — the
// refreshed panel will show the actual state, which is the source of truth.
func obsStreamActionHandler(w http.ResponseWriter, r *http.Request) {
	action := mux.Vars(r)["action"]
	var err error
	switch action {
	case "start":
		err = obsStartStream(r.Context())
	case "stop":
		err = obsStopStream(r.Context())
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "obs stream toggle failed", "action", action, "err", err)
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
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
  /* stream toggle — start (green) or stop (red), with a molly-switch two-click confirm */
  .stream-form { margin:8px 0 12px; }
  button.stream { font:inherit; font-weight:600; padding:9px 18px; border-radius:6px; border:none; cursor:pointer; transition:background .15s, color .15s; }
  button.stream.start { background:#1f6f3e; color:#e8f7ec; }
  button.stream.start:hover { background:#2a8a4d; }
  button.stream.stop  { background:#6a1e1e; color:#fbe6e6; }
  button.stream.stop:hover  { background:#852828; }
  /* armed = first click landed; second click within 5s actually fires */
  button.stream.armed { background:#f85149; color:#fff; box-shadow:0 0 0 2px #f8514980; }
  .stream-unreachable { color:#666; font-size:.9em; font-style:italic; margin:6px 0 0; }
  /* Collapsible "controls" disclosure — defaults closed so the page stays
     calm; click "controls" to reveal stream toggle + future control widgets. */
  details.controls { margin:24px 0 0; }
  details.controls > summary { font-size:.8em; text-transform:uppercase; letter-spacing:.08em; color:#888; cursor:pointer; padding:8px 0; border-top:1px solid #1c1c1c; list-style:none; }
  details.controls > summary::-webkit-details-marker { display:none; }
  details.controls > summary::before { content:"▸ "; color:#666; transition:transform .15s ease; display:inline-block; }
  details.controls[open] > summary::before { content:"▾ "; }
  details.controls > summary:hover { color:#ccc; }
  details.controls h3 { font-size:.8em; text-transform:uppercase; letter-spacing:.08em; color:#888; margin:14px 0 6px; }
  /* per-service restart — small, unobtrusive, lives at the end of each row */
  .row .restart-form { margin:0; }
  button.restart { font:inherit; font-size:.8em; padding:3px 9px; border-radius:4px; border:1px solid #2a2a2a; background:#1a1a1a; color:#888; cursor:pointer; transition:background .15s, color .15s, border-color .15s; }
  button.restart:hover { background:#252525; color:#ccc; border-color:#3a3a3a; }
  button.restart.armed { background:#f85149; color:#fff; border-color:#f85149; box-shadow:0 0 0 2px #f8514980; }
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
      <form class="restart-form" method="post" action="/admin/restart/{{.Name}}">
        <button type="submit" class="restart" data-arm-label="confirm restart" data-original-label="restart" onclick="return armConfirm(this)">restart</button>
      </form>
    </li>
    {{end}}
  </ul>

  <details class="controls">
    <summary>controls</summary>
    <h3>stream</h3>
    {{if .Stream.Reachable}}
      {{if .Stream.Active}}
      <form class="stream-form" method="post" action="/admin/obs/stream/stop">
        <button type="submit" class="stream stop" data-arm-label="click again to confirm stop" data-original-label="stop stream" onclick="return armConfirm(this)">stop stream</button>
      </form>
      {{else}}
      <form class="stream-form" method="post" action="/admin/obs/stream/start">
        <button type="submit" class="stream start" data-arm-label="click again to confirm start" data-original-label="start stream" onclick="return armConfirm(this)">start stream</button>
      </form>
      {{end}}
    {{else}}
    <p class="stream-unreachable">OBS unreachable</p>
    {{end}}
  </details>

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
<script>
// Molly switch: first click arms the button (relabel + redden, 5s window);
// second click within the window lets the form submit. Click-away or timeout
// disarms. No deps — vanilla DOM only.
function armConfirm(btn) {
  if (btn.dataset.armed === '1') {
    return true; // confirm — let the form submit
  }
  btn.dataset.armed = '1';
  btn.classList.add('armed');
  btn.textContent = btn.dataset.armLabel;
  const disarm = () => {
    btn.dataset.armed = '';
    btn.classList.remove('armed');
    btn.textContent = btn.dataset.originalLabel;
    document.removeEventListener('click', outsideClick, true);
  };
  const outsideClick = (e) => { if (e.target !== btn) disarm(); };
  setTimeout(disarm, 5000);
  // capture-phase listener so clicks anywhere else disarm before they bubble
  setTimeout(() => document.addEventListener('click', outsideClick, true), 0);
  return false; // suppress this submit
}
</script>
</body>
</html>`))
