package server

import (
	"context"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
)

// startedAt marks process start so the landing page can report uptime. Set at
// package load; close enough to process start for a human-readable "up Xh".
var startedAt = time.Now()

// healthClient is the short-timeout client used for the sibling-service status
// ping. 2s keeps a slow or hung vlc-server from stalling the landing render.
var healthClient = &http.Client{Timeout: 2 * time.Second}

// grafanaURL points at the Grafana Cloud dashboards list (the TripBot folder
// lives there). Fixed — the org URL doesn't vary by environment.
const grafanaURL = "https://adanalife.grafana.net/dashboards"

// serviceStatus is one row in the landing page's status table.
type serviceStatus struct {
	Name   string
	OK     bool
	Detail string
}

// navLink is one entry in the landing page's links list.
type navLink struct {
	Label string
	URL   string
}

// landingData is the template payload.
type landingData struct {
	Channel  string
	Env      string
	Uptime   string
	Services []serviceStatus
	Links    []navLink
}

// landingHandler serves the human-facing root page on the tripbot Ingress: a
// lightweight status overview (tripbot's own readiness + a live vlc-server
// ping) plus links out to OBS, Grafana, and the Twitch channel. Replaces the
// bare 404 that used to sit on "/".
func landingHandler(w http.ResponseWriter, r *http.Request) {
	data := landingData{
		Channel:  c.Conf.ChannelName,
		Env:      c.Conf.Environment,
		Uptime:   time.Since(startedAt).Round(time.Second).String(),
		Services: gatherStatus(r.Context()),
		Links:    gatherLinks(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := landingTmpl.Execute(w, data); err != nil {
		slog.ErrorContext(r.Context(), "couldn't render landing page", "err", err)
	}
}

// gatherStatus reports tripbot's own readiness (in-memory, free) and pings
// vlc-server's readiness endpoint over the in-cluster Service DNS. The ping is
// best-effort: any failure (DNS, timeout, non-2xx) renders as not-OK rather
// than erroring the page.
func gatherStatus(ctx context.Context) []serviceStatus {
	tripbot := serviceStatus{Name: "tripbot", OK: ready.Load()}
	if tripbot.OK {
		tripbot.Detail = "ready"
	} else {
		tripbot.Detail = "degraded (awaiting Twitch)"
	}

	vlc := serviceStatus{Name: "vlc-server"}
	if c.Conf.VlcServerHost != "" {
		vlc.OK = pingHealthy(ctx, "http://"+c.Conf.VlcServerHost+"/health/ready")
	}
	if vlc.OK {
		vlc.Detail = "healthy"
	} else {
		vlc.Detail = "unreachable"
	}

	return []serviceStatus{tripbot, vlc}
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
// dashboards, and the Twitch channel. Entries whose URL can't be derived are
// dropped rather than rendered broken.
func gatherLinks() []navLink {
	links := []navLink{}
	if obs := siblingURL(c.Conf.ExternalURL, "obs"); obs != "" {
		links = append(links, navLink{Label: "OBS (noVNC)", URL: obs})
	}
	links = append(links, navLink{Label: "Grafana dashboards", URL: grafanaURL})
	if c.Conf.ChannelName != "" {
		links = append(links, navLink{Label: "Twitch channel", URL: "https://twitch.tv/" + c.Conf.ChannelName})
	}
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
  h1 { font-size:20px; margin:0 0 4px; letter-spacing:.02em; }
  .meta { color:#888; margin:0 0 24px; font-size:13px; }
  h2 { font-size:12px; text-transform:uppercase; letter-spacing:.08em; color:#888; margin:24px 0 8px; }
  ul { list-style:none; margin:0; padding:0; }
  .svc { display:flex; align-items:center; gap:10px; padding:6px 0; border-bottom:1px solid #1c1c1c; }
  .svc .name { flex:1; }
  .svc .detail { color:#888; font-size:13px; }
  .dot { width:9px; height:9px; border-radius:50%; flex:0 0 auto; }
  .up { background:#3fb950; box-shadow:0 0 6px #3fb95080; }
  .down { background:#f85149; box-shadow:0 0 6px #f8514980; }
  a { color:#58a6ff; text-decoration:none; display:block; padding:6px 0; border-bottom:1px solid #1c1c1c; }
  a:hover { color:#9cf; }
</style>
</head>
<body>
<main>
  <h1>tripbot</h1>
  <p class="meta">channel <strong>{{.Channel}}</strong> · env {{.Env}} · up {{.Uptime}}</p>

  <h2>status</h2>
  <ul>
    {{range .Services}}
    <li class="svc">
      <span class="dot {{if .OK}}up{{else}}down{{end}}"></span>
      <span class="name">{{.Name}}</span>
      <span class="detail">{{.Detail}}</span>
    </li>
    {{end}}
  </ul>

  <h2>links</h2>
  <ul>
    {{range .Links}}
    <li><a href="{{.URL}}">{{.Label}} →</a></li>
    {{end}}
  </ul>
</main>
</body>
</html>`))
