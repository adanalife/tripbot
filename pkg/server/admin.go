package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/httpmw"
	"github.com/adanalife/tripbot/pkg/obs"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
	"github.com/gorilla/mux"
)

// startedAt marks process start so the admin panel can report uptime. Set at
// package load; close enough to process start for a human-readable "up Xh".
var startedAt = time.Now()

// healthClient is the short-timeout client used for the sibling-service status
// ping. 2s keeps a slow or hung vlc-server from stalling the panel render.
var healthClient = &http.Client{Timeout: 2 * time.Second}

var (
	// chatterCount is the in-memory count of users in chat, refreshed ~60s by
	// the UpdateSession cron. Overridable in tests.
	chatterCount = mytwitch.ChatterCount
	// accountsNeedingReauth reports which Twitch accounts have a missing/expired
	// token; the admin panel renders a re-auth prompt for each. Overridable in
	// tests. Reads in-memory token state — no DB or network call.
	accountsNeedingReauth = mytwitch.AccountsNeedingReauth
	// authStatuses reports every identity's live token state (expiry +
	// reauth reason) for the panel's countdown card. Overridable in tests.
	authStatuses = mytwitch.TokenStatuses
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
	// tailnetBase is Dana's tailnet MagicDNS suffix. App UIs are exposed by
	// the Tailscale K8s operator at <service>-<env>.<tailnetBase>, e.g.
	// tripbot-prod.tail020deb.ts.net. We prefer these over the LAN-IP-backed
	// *.whereisdana.today URLs because the latter silently fail on client
	// networks that share the home LAN's 192.168.1.0/24 range — the operator
	// names are CGNAT (100.x), so they never collide. See vault
	// decisions/tailscale-access-model.md.
	tailnetBase = "tail020deb.ts.net"
	// traefikURL / hubbleBaseURL are the cluster's platform dashboards. They
	// live in kube-system as a single prod-zone install shared across envs
	// (see vault decisions/stage-prod-cotenancy.md), so the host is always
	// -prod regardless of which env's panel is rendering the link.
	traefikURL = "https://traefik-prod." + tailnetBase
	// hubbleBaseURL is the prod-zone Hubble install. gatherLinks appends
	// ?namespace=<env's namespace> so the link lands straight in that
	// namespace's flow view instead of Hubble's "choose a namespace" page —
	// see hubbleNamespace.
	hubbleBaseURL = "https://hubble-prod." + tailnetBase + "/"
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
// SinceUnix is the clip's start time as a Unix timestamp; the page's JS ticker
// counts the elapsed display up from it (and resets it on each live video swap)
// so progress keeps moving without a reload.
type nowPlaying struct {
	File      string
	State     string
	Progress  string
	SinceUnix int64
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

// streamControlTmpl renders the OBS stream toggle widget. It is the swap target
// for the stream action POSTs (hx-target="#stream-control", hx-swap="outerHTML")
// and is rendered identically on initial page load (via the streamControl
// template func) and by obsStreamActionHandler's response, so a live toggle
// matches a fresh page render. When OBS is unreachable the toggle is hidden — a
// click would just fail.
var streamControlTmpl = template.Must(template.New("streamcontrol").Parse(
	`<div id="stream-control" class="stream-control"{{if .OOB}} hx-swap-oob="true"{{end}}>` +
		`{{- if .Reachable}}` +
		`{{- if .Active}}` +
		`<button type="button" class="stream stop" hx-post="/admin/obs/stream/stop" hx-target="#stream-control" hx-swap="outerHTML" hx-disabled-elt="this" data-arm-label="click again to confirm stop" data-original-label="stop stream">stop stream</button>` +
		`{{- else}}` +
		`<button type="button" class="stream start" hx-post="/admin/obs/stream/start" hx-target="#stream-control" hx-swap="outerHTML" hx-disabled-elt="this" data-arm-label="click again to confirm start" data-original-label="start stream">start stream</button>` +
		`{{- end}}` +
		`{{- else}}<p class="stream-unreachable">OBS unreachable</p>{{- end}}` +
		`</div>`))

// renderStreamControl executes streamControlTmpl to a string. oob=true tags the
// widget for an out-of-band swap (used by the periodic /admin/refresh poll);
// false is the inline form used by the initial render (via the streamControl
// template func) and the action response (which targets #stream-control
// directly via hx-target).
func renderStreamControl(sc streamControl, oob bool) string {
	var sb strings.Builder
	data := struct {
		streamControl
		OOB bool
	}{sc, oob}
	if err := streamControlTmpl.Execute(&sb, data); err != nil {
		slog.Error("couldn't render stream control", "err", err)
		return ""
	}
	return sb.String()
}

// statusRowsTmpl renders the <li> service-status rows. Extracted so the initial
// page render (via the statusRows template func) and the /admin/refresh poll
// produce identical markup, mirroring the streamControl/authCard pattern.
var statusRowsTmpl = template.Must(template.New("statusrows").Parse(
	`{{range .}}<li class="row">` +
		`<span class="dot {{if .OK}}up{{else}}down{{end}}"></span>` +
		`<span class="name">{{.Name}}</span>` +
		`{{if .Uptime}}<span class="uptime">up {{.Uptime}}</span>{{end}}` +
		`{{if .Version}}<span class="ver">{{if .VersionURL}}<a href="{{.VersionURL}}">{{.Version}}</a>{{else}}{{.Version}}{{end}}</span>{{end}}` +
		`<span class="status">{{.Detail}}</span>` +
		`<button type="button" class="restart" hx-post="/admin/restart/{{.Name}}" hx-swap="none" hx-disabled-elt="this" data-arm-label="confirm restart" data-original-label="restart">restart</button>` +
		`</li>{{end}}`))

// renderStatusRows executes statusRowsTmpl to a string.
func renderStatusRows(s []serviceStatus) string {
	var sb strings.Builder
	if err := statusRowsTmpl.Execute(&sb, s); err != nil {
		slog.Error("couldn't render status rows", "err", err)
		return ""
	}
	return sb.String()
}

// navLink is one entry in the admin panel's links list.
type navLink struct {
	Label string
	URL   string
}

// featureFlag is one row in the admin panel's "feature flags" section. Stripped
// down from feature.Flag — the template only needs what it renders. The
// target-removal date intentionally isn't displayed (the panel is phone-sized;
// the date is still on feature.Flag for the future admin CRUD UI / audit job).
type featureFlag struct {
	Key         string
	Enabled     bool
	Description string
}

// adminData is the template payload.
type adminData struct {
	Channel        string // broadcaster Twitch username; renders as text + broadcaster link
	PreviewChannel string // Twitch channel the stream-preview embed loads; usually == Channel
	Bot            string // bot Twitch username
	Env            string
	Uptime         string
	Chatters       int // users currently in chat
	Services       []serviceStatus
	Now            *nowPlaying     // nil when vlc is unhealthy or nothing is playing
	Audio          nowPlayingTrack // current SomaFM track; empty Title hides the line
	Stream         streamControl
	PanelHost      string // host the panel was reached at; the Twitch embed needs it as parent=
	Links          []navLink
	Flags          []featureFlag                 // feature flags + their state; empty hides the section
	Reauth         []mytwitch.AccountReauth      // accounts whose token needs re-auth; empty when healthy
	AuthStatuses   []mytwitch.AccountTokenStatus // every identity's token state; drives the live expiry-countdown card
	ChatHistory    []ChatLine                    // recent chat from the live-console hub; live lines stream in via SSE
	MapTrailJSON   string                        // recent GPS breadcrumbs as JSON [[lat,lng],…] for the live map (data attr)
}

// adminHandler serves the human-facing root page on the tripbot Ingress: a
// lightweight status overview (tripbot's own readiness + live sibling-service
// pings, each version linking to its changelog), the currently-playing video
// when vlc is up, the broadcaster/bot accounts, and links to the OBS / Grafana
// / Traefik / Hubble dashboards. Replaces the bare 404 that used to sit on "/".
func (s *Server) adminHandler(w http.ResponseWriter, r *http.Request) {
	vlc := siblingStatus(r.Context(), "vlc", c.Conf.VlcServerHost)
	onscreens := siblingStatus(r.Context(), "onscreens", c.Conf.OnscreensServerHost)
	obs := siblingStatus(r.Context(), "obs", c.Conf.ObsServerHost)

	data := adminData{
		Channel:        c.Conf.ChannelName,
		PreviewChannel: previewChannel(),
		Bot:            c.Conf.BotUsername,
		Env:            c.Conf.Environment,
		Uptime:         time.Since(startedAt).Round(time.Second).String(),
		Chatters:       chatterCount(),
		Services:       s.gatherStatus(buildSHA(), vlc, onscreens, obs),
		Now:            s.currentVideo(vlc.OK),
		Audio:          nowPlayingFetcher(r.Context()),
		Stream:         gatherStream(r.Context()),
		PanelHost:      panelHost(r),
		Links:          gatherLinks(),
		Flags:          s.gatherFlags(r.Context()),
		Reauth:         accountsNeedingReauth(),
		AuthStatuses:   authStatuses(),
		ChatHistory:    s.hub.snapshotChat(),
		MapTrailJSON:   s.mapTrailJSON(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Cache the shell so revisiting (Home Screen reopen, tab return) repaints the
	// last view instantly instead of a white blank while the page loads. The panel
	// is live — SSE plus the 15s /admin/refresh poll reconcile any staleness within
	// seconds — so an aggressive shell cache is safe here. Dropping the old
	// no-store also lets the browser's back/forward cache restore the page (and
	// resume its SSE connection) instantly on tab return; stale-while-revalidate
	// keeps serving the cached paint while a fresh copy loads in the background.
	w.Header().Set("Cache-Control", "max-age=30, stale-while-revalidate=86400")
	if err := adminTmpl.Execute(w, data); err != nil {
		slog.ErrorContext(r.Context(), "couldn't render admin panel", "err", err)
	}
}

// gatherStatus reports tripbot's own readiness (in-memory, free) and folds in
// the already-probed sibling-service rows. Each row carries its build tag in a
// version column linking to that build's changelog at the deployed sha.
func (s *Server) gatherStatus(sha string, siblings ...serviceStatus) []serviceStatus {
	tripbot := serviceStatus{
		Name:       "tripbot",
		OK:         s.twitchConnected.Load(),
		Version:    s.versionTag,
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

// gatherFlags reads the FlagClient's last-known snapshot and shapes each
// flag into the small display row the template renders. Returns nil when
// no flags are loaded yet (startup window before SetFlagClient) so the
// template's {{if .Flags}} hides the section cleanly.
func (s *Server) gatherFlags(ctx context.Context) []featureFlag {
	flags := s.flagSnapshot(ctx)
	if len(flags) == 0 {
		return nil
	}
	out := make([]featureFlag, 0, len(flags))
	for _, f := range flags {
		out = append(out, featureFlag{
			Key:         f.Key,
			Enabled:     f.Enabled,
			Description: f.Description,
		})
	}
	return out
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
	case "vlc":
		err = restartProxyShutdown(r.Context(), c.Conf.VlcServerHost)
	case "onscreens":
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
	// The button is hx-swap="none" — no body to return. Immediate feedback is the
	// in-flight button state; the service drops then reappears in the status rows
	// on the next panel refresh.
	w.WriteHeader(http.StatusNoContent)
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
	// Render from the resulting state: a successful start/stop took effect; a
	// failed one left the previous state, so the swapped-in widget simply doesn't
	// flip. The panel's periodic refresh reconciles any drift from OBS state
	// changed outside the panel. (Source-of-truth philosophy, minus the full-page
	// reload the old 303 redirect forced.)
	active := (action == "start") == (err == nil)
	// HX-Trigger lets the page react beyond the swapped widget — here, open or
	// close the stream-preview disclosure to match the new state.
	w.Header().Set("HX-Trigger", fmt.Sprintf(`{"tripbot:stream-changed":{"active":%t}}`, active))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, werr := w.Write([]byte(renderStreamControl(streamControl{Reachable: true, Active: active}, false))); werr != nil {
		slog.ErrorContext(r.Context(), "couldn't write stream control", "err", werr)
	}
}

// refreshHandler serves GET /admin/refresh: the always-present live panel
// sections — the service status rows and the OBS stream toggle — as out-of-band
// fragments. A hidden poller on the page (hx-trigger="every 15s") fetches this,
// so the up/down dots, uptimes, versions, and stream state stay current without
// a full reload. Conditionally-rendered sections (now-playing audio, feature
// flags) are intentionally excluded: OOB-swapping into a target that may not be
// in the DOM is fragile, and those values are low-volatility — they refresh on
// the next full page load. Same sibling-ping cost as the root render; fine at
// 15s for a single-operator panel.
func (s *Server) refreshHandler(w http.ResponseWriter, r *http.Request) {
	vlc := siblingStatus(r.Context(), "vlc", c.Conf.VlcServerHost)
	onscreens := siblingStatus(r.Context(), "onscreens", c.Conf.OnscreensServerHost)
	obs := siblingStatus(r.Context(), "obs", c.Conf.ObsServerHost)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	var sb strings.Builder
	sb.WriteString(`<ul id="status-list" hx-swap-oob="true">`)
	sb.WriteString(renderStatusRows(s.gatherStatus(buildSHA(), vlc, onscreens, obs)))
	sb.WriteString(`</ul>`)
	sb.WriteString(renderStreamControl(gatherStream(r.Context()), true))
	if _, err := w.Write([]byte(sb.String())); err != nil {
		slog.ErrorContext(r.Context(), "couldn't write panel refresh", "err", err)
	}
}

// currentVideo summarizes the currently-playing video for the page, but only
// when vlc is healthy (a stale value while vlc is down would be misleading).
// Reads the last video.changed cached by the hub from NATS — no in-process
// pkg/video dependency — so the panel can later be lifted into its own service.
// Returns nil when no video.changed has arrived yet. Progress is derived from
// the event's emitted_at (the clip start), matching the live SSE ticker.
func (s *Server) currentVideo(vlcOK bool) *nowPlaying {
	if !vlcOK {
		return nil
	}
	ev, ok := s.hub.snapshotNowPlaying()
	if !ok || ev.File == "" {
		return nil
	}
	started := parseEmitted(ev.EmittedAt)
	return &nowPlaying{
		File:      ev.File,
		State:     ev.State,
		Progress:  time.Since(started).Round(time.Second).String(),
		SinceUnix: started.Unix(),
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
	if obs := tailnetServiceURL("obs"); obs != "" {
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

// tailnetServiceURL returns the Tailscale K8s-operator URL for a sibling
// service in this env's namespace, e.g. obs-prod.tail020deb.ts.net for
// production. Tailnet (CGNAT 100.x) URLs are preferred over the LAN-IP-backed
// *.whereisdana.today URLs because the latter silently fail on client
// networks that overlap the home LAN's 192.168.1.0/24 range. Returns "" for
// environments not served by the operator (dev/local/testing run on a
// separate cluster), which hides the link rather than rendering one that
// won't reach anything.
func tailnetServiceURL(service string) string {
	var env string
	switch {
	case c.Conf.IsProduction():
		env = "prod"
	case c.Conf.IsStaging():
		env = "stage"
	default:
		return ""
	}
	return "https://" + service + "-" + env + "." + tailnetBase
}

// previewChannel returns the Twitch channel name the stream-preview embed
// should load.
func previewChannel() string {
	return c.Conf.ChannelName
}

// envColorClass returns the CSS modifier suffix for the env-badge chip,
// keyed off c.Conf.Environment (same source OTLP's deployment.environment
// reads). The empty string falls back to the neutral chip styling. Kept as
// a template helper so the colour map lives next to the env-source lookup
// rather than inside the inline-template string.
func envColorClass(env string) string {
	switch env {
	case "production":
		return "env-prod"
	case "staging":
		return "env-stage"
	case "development":
		return "env-dev"
	default:
		return ""
	}
}

// panelHost returns the hostname the panel was reached at, for use as the
// Twitch embed's parent= parameter. Read from r.Host (the request's Host
// header) so it matches whatever the browser used — Tailscale's tail*.ts.net
// hostname when the panel is reached via the operator's tailnet, the public
// FQDN when reached through the Ingress. Strips any port suffix. Returns ""
// when r.Host is empty (unusual; template hides the stream-preview block).
func panelHost(r *http.Request) string {
	h := r.Host
	if h == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(h); err == nil {
		return host
	}
	return h
}

var adminTmpl = template.Must(template.New("admin").Funcs(template.FuncMap{
	"envColorClass": envColorClass,
	// authCard / reauthCallout render the same fragments the hub pushes live,
	// so the initial server render and a later SSE swap are byte-identical.
	"authCard":      func(s []mytwitch.AccountTokenStatus) template.HTML { return template.HTML(renderAuthCard(s)) },
	"reauthCallout": func(r []mytwitch.AccountReauth) template.HTML { return template.HTML(renderReauthCallout(r)) },
	// streamControl renders the OBS toggle so the initial paint is byte-identical
	// to what obsStreamActionHandler swaps in after a live toggle.
	"streamControl": func(sc streamControl) template.HTML { return template.HTML(renderStreamControl(sc, false)) },
	// statusRows renders the service-status <li> rows, shared with /admin/refresh.
	"statusRows": func(s []serviceStatus) template.HTML { return template.HTML(renderStatusRows(s)) },
}).Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>tripbot — {{.Channel}} ({{.Env}})</title>
<!-- favicons referenced from the website (single owner of brand assets) — see
     vault general/logo.md; apple-touch-icon gives a home-screen icon on phones -->
<link rel="icon" type="image/png" sizes="32x32" href="https://www.dana.lol/assets/favicon-32x32.png">
<link rel="icon" type="image/png" sizes="16x16" href="https://www.dana.lol/assets/favicon-16x16.png">
<link rel="apple-touch-icon" sizes="180x180" href="https://www.dana.lol/assets/apple-touch-icon.png">
<!-- htmx + its SSE extension, vendored + embedded (pkg/server/static) — drives
     the live chat console: sse.js consumes /admin/events and swaps fragments. -->
<script src="/static/htmx.min.js"></script>
<script src="/static/sse.js"></script>
<!-- Leaflet (vendored + embedded) for the live location map. Map tiles come
     from OpenStreetMap at render time (no API key); the marker is an emoji
     divIcon so no marker images are needed. -->
<link rel="stylesheet" href="/static/leaflet.css">
<script src="/static/leaflet.js"></script>
<style>
  :root {
    color-scheme: dark light;
    --mono: ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;
    /* Dark palette (default + user-override) */
    --bg:#0a0a0a; --fg:#eee; --muted:#888; --dim:#666; --faint:#cdd;
    --chip-bg:#1a1a1a; --chip-border:#262626;
    --divider:#1c1c1c; --logo-invert:1;
  }
  @media (prefers-color-scheme: light) {
    :root:not([data-theme="dark"]) {
      color-scheme: light;
      --bg:#fafaf7; --fg:#111; --muted:#666; --dim:#888; --faint:#444;
      --chip-bg:#ececea; --chip-border:#d8d8d4;
      --divider:#e2e2de; --logo-invert:0;
    }
  }
  :root[data-theme="light"] {
    color-scheme: light;
    --bg:#fafaf7; --fg:#111; --muted:#666; --dim:#888; --faint:#444;
    --chip-bg:#ececea; --chip-border:#d8d8d4;
    --divider:#e2e2de; --logo-invert:0;
  }
  body { background:var(--bg); color:var(--fg); font:clamp(14px,0.5vw + 11px,18px)/1.65 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif; margin:0; display:flex; min-height:100vh; align-items:center; justify-content:center; }
  main { width:min(92vw,520px); padding:clamp(24px,4vw,48px); }
  /* logo is the monochrome black mark; invert to white on dark bg, leave as-is on light */
  .logo { width:clamp(44px,5vw,60px); height:auto; filter:invert(var(--logo-invert)); opacity:.92; display:block; margin:0 0 16px; }
  .logo-link { display:inline-block; }
  .logo-link:hover .logo { opacity:1; }
  h1 { font-size:clamp(20px,1.2vw + 15px,28px); margin:0 0 4px; letter-spacing:.02em; }
  .env { font-family:var(--mono); background:var(--chip-bg); border:1px solid var(--chip-border); color:var(--faint); padding:2px 7px; border-radius:5px; font-size:.5em; font-weight:normal; letter-spacing:0; vertical-align:middle; }
  /* env-badge colour variants — keyed off c.Conf.Environment (same source
     OTLP's deployment.environment reads). Greens/yellows/blues are picked
     to stay legible against both the dark and light --chip-bg defaults;
     unknown envs fall through to the neutral chip styling above. */
  .env.env-prod  { background:#0f3a1f; border-color:#1f6f3e; color:#a4e0b8; }
  .env.env-stage { background:#3a2f0a; border-color:#7a5e1a; color:#f0d57d; }
  .env.env-dev   { background:#0f2a45; border-color:#1f548a; color:#9ec7f0; }
  @media (prefers-color-scheme: light) {
    :root:not([data-theme="dark"]) .env.env-prod  { background:#dff5e4; border-color:#7fc296; color:#15532a; }
    :root:not([data-theme="dark"]) .env.env-stage { background:#fbf0c8; border-color:#d4b35a; color:#5a4310; }
    :root:not([data-theme="dark"]) .env.env-dev   { background:#dbe9f7; border-color:#7aa6d1; color:#16365a; }
  }
  :root[data-theme="light"] .env.env-prod  { background:#dff5e4; border-color:#7fc296; color:#15532a; }
  :root[data-theme="light"] .env.env-stage { background:#fbf0c8; border-color:#d4b35a; color:#5a4310; }
  :root[data-theme="light"] .env.env-dev   { background:#dbe9f7; border-color:#7aa6d1; color:#16365a; }
  .meta { color:var(--muted); margin:0 0 2px; font-size:.92em; }
  .accounts { color:var(--dim); margin:0 0 24px; font-size:.85em; }
  h2 { font-size:.8em; text-transform:uppercase; letter-spacing:.08em; color:var(--muted); margin:24px 0 8px; }
  ul { list-style:none; margin:0; padding:0; }
  .row { display:flex; align-items:center; gap:12px; padding:7px 0; border-bottom:1px solid var(--divider); }
  .row .name { flex:1; }
  .row .uptime { font-size:.85em; color:var(--dim); }
  .row .ver { font-family:var(--mono); font-size:.85em; color:var(--dim); }
  /* status hugs the far right and right-aligns so it forms a clean column,
     aligned whether or not the row carries a version */
  .row .status { color:var(--muted); font-size:.92em; text-align:right; min-width:5.5em; }
  .dot { width:9px; height:9px; border-radius:50%; flex:0 0 auto; }
  .up { background:#3fb950; box-shadow:0 0 6px #3fb95080; }
  .down { background:#f85149; box-shadow:0 0 6px #f8514980; }
  .now { margin:0; padding:7px 0; }
  .now .file { font-family:var(--mono); word-break:break-all; }
  .now .state { color:var(--muted); }
  /* audio = the SomaFM background track from gsclassic.json */
  .audio { margin:0; padding:7px 0; }
  .audio .track { color:var(--fg); opacity:.85; }
  .audio .state { color:var(--muted); }
  /* stream-preview disclosure — same shape as .controls, just a different
     summary label. iframe is lazy-loaded by the inline JS so the ~2MB
     Twitch player isn't fetched on every panel render. */
  /* Shared disclosure-row styling — controls / now-playing / stream-preview
     all default closed and reveal on click. Same look, just different
     summary labels. */
  details.stream-preview, details.now-playing, details.feature-flags, details.controls { margin:24px 0 0; }
  details.stream-preview > summary, details.now-playing > summary, details.feature-flags > summary, details.controls > summary { font-size:.8em; text-transform:uppercase; letter-spacing:.08em; color:var(--muted); cursor:pointer; padding:8px 0; border-top:1px solid var(--divider); list-style:none; }
  details.stream-preview > summary::-webkit-details-marker, details.now-playing > summary::-webkit-details-marker, details.feature-flags > summary::-webkit-details-marker, details.controls > summary::-webkit-details-marker { display:none; }
  details.stream-preview > summary::before, details.now-playing > summary::before, details.feature-flags > summary::before, details.controls > summary::before { content:"▸ "; color:var(--dim); display:inline-block; }
  details.stream-preview[open] > summary::before, details.now-playing[open] > summary::before, details.feature-flags[open] > summary::before, details.controls[open] > summary::before { content:"▾ "; }
  details.stream-preview > summary:hover, details.now-playing > summary:hover, details.feature-flags > summary:hover, details.controls > summary:hover { color:var(--fg); }
  /* feature-flags rows reuse .row but the name column carries the flag key
     in monospace; description trails after a separator dot like elsewhere. */
  .row.flag-row .name code { font-family:var(--mono); font-size:.95em; }
  .stream-frame { aspect-ratio:16 / 9; width:100%; margin-top:10px; background:#000; border-radius:6px; overflow:hidden; }
  .stream-frame iframe { width:100%; height:100%; border:none; display:block; }
  a { color:#58a6ff; text-decoration:none; }
  a:hover { color:#9cf; }
  /* theme toggle — text-only, sits to the right of the env chip */
  .panel-footer { margin-top:32px; padding-top:14px; border-top:1px solid var(--divider); display:flex; gap:10px; align-items:center; }
  .theme-toggle { background:none; border:none; color:var(--dim); font:inherit; font-size:.85em; cursor:pointer; padding:2px 6px; }
  .theme-toggle:hover { color:var(--fg); }
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
  .stream-control { margin:8px 0 12px; }
  button.stream { font:inherit; font-size:.9em; padding:6px 14px; border-radius:5px; border:none; cursor:pointer; transition:background .15s, color .15s; }
  button.stream.start { background:#1f6f3e; color:#e8f7ec; font-weight:600; }
  button.stream.start:hover { background:#2a8a4d; }
  /* "stop stream" is the less-likely-correct click — render it muted so
     it doesn't dominate. The molly-switch arms it red on first click. */
  button.stream.stop  { background:#2a1a1a; color:#c89696; border:1px solid #4a2a2a; }
  button.stream.stop:hover  { background:#3a2020; color:#e8b0b0; }
  /* armed = first click landed; second click within 5s actually fires */
  button.stream.armed { background:#f85149; color:#fff; box-shadow:0 0 0 2px #f8514980; }
  .stream-unreachable { color:#666; font-size:.9em; font-style:italic; margin:6px 0 0; }
  /* Collapsible "controls" disclosure shares the disclosure-row styling
     above (folded into the .stream-preview/.now-playing multi-selector);
     only the .controls-specific h3 sub-heading lives here. */
  details.controls h3 { font-size:.8em; text-transform:uppercase; letter-spacing:.08em; color:var(--muted); margin:14px 0 6px; }
  /* per-service restart — small, unobtrusive, lives at the end of each row */
  button.restart { font:inherit; font-size:.8em; padding:3px 9px; border-radius:4px; border:1px solid #2a2a2a; background:#1a1a1a; color:#888; cursor:pointer; transition:background .15s, color .15s, border-color .15s; }
  button.restart:hover { background:#252525; color:#ccc; border-color:#3a3a3a; }
  button.restart.armed { background:#f85149; color:#fff; border-color:#f85149; box-shadow:0 0 0 2px #f8514980; }
  /* in-flight feedback: htmx adds .htmx-request to an action button while its
     hx-post is in flight, and hx-disabled-elt also sets :disabled. Dim it + show
     a progress cursor so a tap reads as "working" and a double-tap is visibly
     inert until the request settles. */
  button.htmx-request, button.stream:disabled, button.restart:disabled { opacity:.55; cursor:progress; }
  /* live chat console — recent history rendered server-side, live lines stream
     in via SSE (sse-swap="chat", beforeend). Same disclosure look as the others. */
  details.chat { margin:24px 0 0; }
  details.chat > summary { font-size:.8em; text-transform:uppercase; letter-spacing:.08em; color:var(--muted); cursor:pointer; padding:8px 0; border-top:1px solid var(--divider); list-style:none; }
  details.chat > summary::-webkit-details-marker { display:none; }
  details.chat > summary::before { content:"▸ "; color:var(--dim); display:inline-block; }
  details.chat[open] > summary::before { content:"▾ "; }
  details.chat > summary:hover { color:var(--fg); }
  .chat-log { max-height:40vh; overflow-y:auto; margin-top:8px; font-size:.92em; line-height:1.5; overscroll-behavior:contain; }
  .chat-line { padding:2px 0; word-break:break-word; }
  .chat-line .ct-ts { color:var(--dim); font-family:var(--mono); font-size:.8em; margin-right:4px; }
  .chat-line .cu { color:#58a6ff; font-weight:600; }
  .chat-line .ct { color:var(--fg); }
  .chat-empty { color:var(--dim); font-style:italic; padding:6px 0; }
  /* jump-to-latest pill — floats over the bottom of the chat-log, shown only
     while scrolled up (the scrollback buffer). Tapping returns to the newest. */
  .chat-wrap { position:relative; }
  .chat-jump { position:absolute; left:50%; bottom:10px; transform:translateX(-50%); z-index:2; font:inherit; font-size:.8em; padding:4px 12px; border-radius:999px; border:1px solid var(--chip-border); background:var(--chip-bg); color:var(--fg); cursor:pointer; box-shadow:0 2px 8px rgba(0,0,0,.35); }
  .chat-jump:hover { border-color:#58a6ff; color:#9cf; }
  .chat-jump[hidden] { display:none; }
  /* clickable chat usernames + the profile popover they open */
  .chat-line .cu { cursor:pointer; }
  .chat-line .cu:hover { text-decoration:underline; }
  .user-popover { position:fixed; z-index:20; max-width:240px; background:var(--chip-bg); border:1px solid var(--chip-border); border-radius:8px; padding:12px 14px; box-shadow:0 6px 24px rgba(0,0,0,.45); font-size:.9em; }
  .user-popover[hidden] { display:none; }
  .profile-card .profile-name { font-weight:600; margin-bottom:8px; word-break:break-all; }
  .profile-bot { font-size:.72em; color:var(--dim); border:1px solid var(--chip-border); border-radius:4px; padding:0 5px; vertical-align:middle; }
  .profile-stats { display:grid; grid-template-columns:auto 1fr; gap:2px 14px; margin:0 0 8px; }
  .profile-stats dt { color:var(--muted); }
  .profile-stats dd { margin:0; text-align:right; font-variant-numeric:tabular-nums; }
  .profile-empty { color:var(--dim); font-style:italic; margin:0 0 8px; }
  .profile-link { font-size:.85em; }
  /* live location map (Leaflet) */
  details.map { margin:24px 0 0; }
  details.map > summary { font-size:.8em; text-transform:uppercase; letter-spacing:.08em; color:var(--muted); cursor:pointer; padding:8px 0; border-top:1px solid var(--divider); list-style:none; }
  details.map > summary::-webkit-details-marker { display:none; }
  details.map > summary::before { content:"▸ "; color:var(--dim); display:inline-block; }
  details.map[open] > summary::before { content:"▾ "; }
  details.map > summary:hover { color:var(--fg); }
  .map-box { height:280px; margin-top:8px; border-radius:8px; overflow:hidden; background:var(--chip-bg); }
  .van-marker { font-size:22px; line-height:24px; text-align:center; }
  .map-box .leaflet-control-attribution { font-size:.65em; }
  .map-toggle { margin-top:8px; background:none; border:none; color:var(--dim); font:inherit; font-size:.85em; cursor:pointer; padding:2px 6px; }
  .map-toggle:hover { color:var(--fg); }
  /* live viewer count — a subtle, quick colour flash on the number: green when
     it rises, red when it falls. The inner .chatters-count span is re-inserted
     on each SSE update so the animation re-triggers; with only a "from" keyframe
     it animates back to the number's natural colour. Steady state is unstyled. */
  @keyframes flash-up   { from { color:#3fb950; } }
  @keyframes flash-down { from { color:#f85149; } }
  .chatters-count.flash-up   { animation:flash-up .8s ease-out; }
  .chatters-count.flash-down { animation:flash-down .8s ease-out; }
  /* live auth/token card — compact muted line under the accounts line. Each
     identity shows "expires in N" (JS-filled from data-expires); .auth-soon
     reddens it under the threshold, .auth-expired once past, and .auth-warn
     marks a missing/expired token whose row carries a re-auth link instead. */
  .auth { color:var(--dim); margin:0 0 18px; font-size:.82em; }
  .auth-row { margin-right:12px; white-space:nowrap; }
  .auth-who { color:var(--muted); }
  .auth-expires.auth-soon { color:#f0883e; font-weight:600; }
  .auth-expires.auth-expired { color:#f85149; font-weight:600; }
  .auth-row.auth-warn .auth-who { color:#f0883e; }
  a.auth-reauth { color:#ffb454; font-weight:600; }
</style>
</head>
<body>
<!-- hx-ext="sse" + sse-connect open ONE EventSource on /admin/events for the
     whole panel; every live region below (chat, viewer count, …) is an
     sse-swap target nested under it. -->
<main hx-ext="sse" sse-connect="/admin/events">
  <!-- A Dana Life mark, referenced from the website (the single owner of brand
       assets) rather than copied in — see vault general/logo.md. The anchor
       wraps the mark so clicking it refreshes the page. -->
  <a class="logo-link" href="/" title="refresh"><img class="logo" src="https://www.dana.lol/assets/logo.png" alt="A Dana Life" width="44" height="44"></a>
  <h1>tripbot <code class="env {{envColorClass .Env}}">{{.Env}}</code></h1>
  <!-- #chatters is the stable sse-swap target; the inner .chatters-count span is
       replaced (innerHTML) on each "viewers" event so its flash animation re-fires. -->
  <p class="meta">up {{.Uptime}} · <span id="chatters" sse-swap="viewers" hx-swap="innerHTML"><span class="chatters-count">{{.Chatters}}</span></span> in chat</p>
  <p class="accounts">broadcaster <a href="https://twitch.tv/{{.Channel}}">{{.Channel}}</a> · bot <a href="https://twitch.tv/{{.Bot}}">{{.Bot}}</a></p>
  <!-- live token-expiry card: per-identity "expires in N" countdown (the JS
       ticker counts it down + reddens it near expiry), or a re-auth link when a
       token is missing/expired. The hub's 30s poller OOB-swaps #auth-card;
       authCard renders the same fragment here for the initial paint. -->
  <p class="auth" id="auth-card" sse-swap="auth" hx-swap="innerHTML">{{authCard .AuthStatuses}}</p>

  <!-- #reauth-card fills (banner appears) or empties (banner disappears) live as
       the hub's auth poller pushes "reauth" events; reauthCallout renders the
       same markup for the initial paint. -->
  <div id="reauth-card" sse-swap="reauth" hx-swap="innerHTML">{{reauthCallout .Reauth}}</div>

  <h2>status</h2>
  <!-- #status-list is OOB-swapped by the /admin/refresh poll (see the poller near
       the footer) so the up/down dots, uptimes, and versions stay current without
       a full reload. Each row's restart button is hx-post (hx-swap="none" — the
       service just reappears in these rows on the next refresh). -->
  <ul id="status-list">{{statusRows .Services}}</ul>

  <details class="chat" open>
    <summary>chat</summary>
    <!-- #chat-log receives the "chat" SSE event (the panel-wide sse-connect lives
         on <main>) and appends each rendered line. Recent history is rendered
         server-side from the hub's ring buffer; live lines stream in on top. -->
    <div class="chat-wrap">
      <div id="chat-log" class="chat-log" sse-swap="chat" hx-swap="beforeend">
        {{range .ChatHistory}}<div class="chat-line"><time class="ct-ts" datetime="{{.At.Format "2006-01-02T15:04:05Z07:00"}}">{{.At.Format "15:04"}}</time> <span class="cu" hx-get="/admin/user/{{.Username}}" hx-target="#user-popover" hx-swap="innerHTML" hx-trigger="click">{{.Username}}</span> <span class="ct">{{.Text}}</span></div>{{else}}<div class="chat-empty">waiting for chat…</div>{{end}}
      </div>
      <!-- shown only while scrolled up (see JS): tapping jumps to the newest line -->
      <button id="chat-jump" class="chat-jump" type="button" hidden>↓ new</button>
    </div>
  </details>

  {{if and .PreviewChannel .PanelHost}}
  <details class="stream-preview" id="stream-preview" {{if .Stream.Active}}open{{end}}>
    <summary>stream preview</summary>
    <div class="stream-frame">
      <iframe data-src="https://player.twitch.tv/?channel={{.PreviewChannel}}&parent={{.PanelHost}}&muted=true&autoplay=true"
              allowfullscreen
              title="Twitch stream preview"></iframe>
    </div>
  </details>
  {{end}}

  {{if .Flags}}
  <details class="feature-flags">
    <summary>feature flags</summary>
    <ul class="flags">
      {{range .Flags}}
      <li class="row flag-row">
        <span class="dot {{if .Enabled}}up{{else}}down{{end}}"></span>
        <span class="name"><code>{{.Key}}</code>{{if .Description}}<span class="state"> · {{.Description}}</span>{{end}}</span>
        <span class="status">{{if .Enabled}}on{{else}}off{{end}}</span>
      </li>
      {{end}}
    </ul>
  </details>
  {{end}}

  {{if or .Now .Audio.Title}}
  <details class="now-playing">
    <summary>now playing</summary>
    <!-- #now-line is the sse-swap target for "video" events; its inner markup
         mirrors hub.go's videoLineTmpl so a live swap matches a fresh render.
         The .now-elapsed span is counted up by the page's JS ticker. -->
    {{with .Now}}
    <p class="now" id="now-line" sse-swap="video" hx-swap="innerHTML"><span class="file">{{.File}}</span>{{if .State}} <span class="state">· {{.State}}</span>{{end}} <span class="state">· <span class="now-elapsed" data-since="{{.SinceUnix}}">{{.Progress}}</span></span></p>
    {{end}}
    {{if .Audio.Title}}
    <p class="audio">
      <span class="track">{{.Audio.Artist}} — {{.Audio.Title}}</span>
    </p>
    {{end}}
  </details>
  {{end}}

  <details class="map" open>
    <summary>map</summary>
    <!-- data-trail seeds the breadcrumb polyline + 🚐 pin from the hub on load;
         #map-sink receives live "map" SSE points (read on afterSwap by the JS). -->
    <div id="map" class="map-box" data-trail="{{.MapTrailJSON}}"></div>
    <div id="map-sink" sse-swap="map" hx-swap="innerHTML" hidden></div>
    <button id="corpus-toggle" class="map-toggle" type="button">show full route</button>
  </details>

  <details class="controls">
    <summary>controls</summary>
    <h3>stream</h3>
    <!-- stream toggle: hx-post flips OBS streaming and swaps this widget in place
         (no full-page reload). The response re-renders from the resulting state,
         so a failed toggle simply leaves the button unchanged. -->
    {{streamControl .Stream}}
  </details>

  <h2>dashboards</h2>
  <nav class="links">
    {{range .Links}}<a href="{{.URL}}">{{.Label}}</a>{{end}}
  </nav>

  <div class="panel-footer">
    <button id="toggle-all" class="theme-toggle" type="button" title="expand or collapse all sections">▾ expand all</button>
    <button id="theme-toggle" class="theme-toggle" type="button" title="toggle theme">◐ theme</button>
  </div>

  <!-- live refresh: every 15s, fetch the always-present panel sections (service
       status rows + the OBS stream toggle) and OOB-swap them in, so the dots and
       stream state stay current without a full reload. Conditionally-rendered
       sections (now-playing audio, feature flags) refresh on full page load. -->
  <div hx-get="/admin/refresh" hx-trigger="every 15s" hx-swap="none"></div>

  <!-- chat user-profile popover: filled by a fetch when a username in the chat
       console is clicked (see the profile JS); position:fixed so it floats. -->
  <div id="user-popover" class="user-popover" hidden></div>
</main>
<script>
// Stream-preview iframe wiring. The disclosure starts open only when the
// stream is live (collapsed when offline); the initial paint sets src from
// data-src when open. Collapse swaps src to about:blank so the player stops
// cleanly (audio + bandwidth would otherwise keep going), and re-expanding
// restores it.
(function() {
  const details = document.getElementById('stream-preview');
  if (!details) return;
  const iframe = details.querySelector('iframe');
  if (!iframe) return;
  const load = () => { iframe.src = iframe.dataset.src; };
  const unload = () => { iframe.src = 'about:blank'; };
  if (details.open) load();
  details.addEventListener('toggle', () => {
    if (details.open) load(); else unload();
  });
})();

// Expand/collapse all <details> on the page. If any section is closed, the
// click opens them all; if all are already open, it closes them all. Button
// label flips to match the next action. The stream-preview's toggle listener
// (see above) lazy-loads/unloads the iframe on each open/close, so this
// reuses that path correctly.
(function() {
  const btn = document.getElementById('toggle-all');
  if (!btn) return;
  const sync = () => {
    const all = document.querySelectorAll('main details');
    const anyClosed = Array.from(all).some(d => !d.open);
    btn.textContent = anyClosed ? '▾ expand all' : '▸ collapse all';
  };
  btn.addEventListener('click', () => {
    const all = document.querySelectorAll('main details');
    const anyClosed = Array.from(all).some(d => !d.open);
    all.forEach(d => { d.open = anyClosed; });
    sync();
  });
  // Keep the label honest if the operator toggles individual sections.
  document.querySelectorAll('main details').forEach(d => d.addEventListener('toggle', sync));
  sync();
})();

// Theme toggle. Reads localStorage for an explicit user override; otherwise
// the @media(prefers-color-scheme: light) CSS rule applies. Setting
// data-theme on <html> overrides the media query in either direction.
(function() {
  const root = document.documentElement;
  const saved = localStorage.getItem('admin-theme'); // "light" | "dark" | null
  if (saved) root.setAttribute('data-theme', saved);
  const btn = document.getElementById('theme-toggle');
  if (btn) btn.addEventListener('click', () => {
    // Effective theme: the resolved color-scheme (after CSS + override).
    const cur = getComputedStyle(root).colorScheme.includes('light') ? 'light' : 'dark';
    const next = cur === 'light' ? 'dark' : 'light';
    root.setAttribute('data-theme', next);
    localStorage.setItem('admin-theme', next);
  });
})();

// Live chat console. New lines stream into #chat-log (htmx sse, beforeend).
// Auto-scroll + trimming happen ONLY while the viewer is pinned to the bottom —
// scrolling up to read history is never yanked back down and the older nodes
// aren't trimmed out from under the viewport, which is what makes the scrollback
// buffer usable. It re-follows + re-trims once the viewer returns to the bottom.
// Also drops the "waiting for chat…" placeholder and decorates each line
// (local-time + per-user color).
(function() {
  const log = document.getElementById('chat-log');
  if (!log) return;
  const CAP = 500; // scrollback depth; matches chatRingSize on the server
  // pinned = the viewport is at (or near) the newest line.
  let pinned = true;
  const atBottom = () => log.scrollHeight - log.scrollTop - log.clientHeight < 40;
  // Jump-to-latest pill: shown only while scrolled up, counting unread lines.
  const jump = document.getElementById('chat-jump');
  let unread = 0;
  const refreshPill = () => {
    if (!jump) return;
    jump.textContent = '↓ ' + unread + ' new';
    jump.hidden = unread === 0;
  };
  const toBottom = () => { log.scrollTop = log.scrollHeight; pinned = true; unread = 0; refreshPill(); };
  if (jump) jump.addEventListener('click', toBottom);
  log.addEventListener('scroll', () => {
    pinned = atBottom();
    if (pinned) { unread = 0; refreshPill(); }
  });
  // Times are emitted in UTC and rendered server-side as a fallback; show them
  // in the viewer's local timezone instead.
  const localize = (root) => root.querySelectorAll('time.ct-ts').forEach(t => {
    const iso = t.getAttribute('datetime');
    if (!iso) return;
    const d = new Date(iso);
    if (!isNaN(d.getTime())) t.textContent = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  });
  // Give each username a stable color derived from a hash of the name, so the
  // same chatter is always the same hue. L/S tuned to stay legible on both the
  // dark and light panel themes.
  const colorFor = (name) => {
    let h = 0;
    for (let i = 0; i < name.length; i++) h = (Math.imul(h, 31) + name.charCodeAt(i)) >>> 0;
    return 'hsl(' + (h % 360) + ' 65% 60%)';
  };
  const colorize = (root) => root.querySelectorAll('.chat-line .cu').forEach(el => {
    if (!el.dataset.colored) { el.style.color = colorFor(el.textContent); el.dataset.colored = '1'; }
  });
  const decorate = (root) => { localize(root); colorize(root); };
  log.addEventListener('htmx:afterSwap', () => {
    log.querySelectorAll('.chat-empty').forEach(el => el.remove());
    decorate(log);
    if (pinned) {
      while (log.childElementCount > CAP) log.removeChild(log.firstElementChild);
      log.scrollTop = log.scrollHeight;
    } else {
      unread++;
      refreshPill();
    }
  });
  decorate(log); // localize times + color usernames in the server-rendered history
  log.scrollTop = log.scrollHeight; // start at the newest message
})();

// "Now playing" elapsed timer. Each .now-elapsed span carries data-since (the
// clip's start as a Unix timestamp); tick it up once a second so the progress
// keeps moving without a reload. A live "video" swap replaces the span with a
// fresh data-since, so the count resets to the new clip automatically.
(function() {
  const fmt = (s) => {
    s = Math.max(0, Math.floor(s));
    const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), sec = s % 60;
    return (h ? h + 'h' : '') + (h || m ? m + 'm' : '') + sec + 's';
  };
  const tick = () => {
    const now = Date.now() / 1000;
    document.querySelectorAll('.now-elapsed[data-since]').forEach(el => {
      const since = Number(el.dataset.since);
      if (since) el.textContent = fmt(now - since);
    });
  };
  tick();
  setInterval(tick, 1000);
})();

// Auth token countdown. Each .auth-expires carries data-expires (Unix seconds);
// show "expires in N" counting down, redden it under 15 min (.auth-soon) and
// once past (.auth-expired). The hub re-pushes the card every 30s, but this
// keeps the displayed remaining fresh between pushes.
(function() {
  const SOON = 15 * 60; // seconds — redden threshold
  const fmt = (s) => {
    if (s <= 0) return 'expired';
    const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60);
    if (h) return 'expires in ' + h + 'h ' + m + 'm';
    if (m) return 'expires in ' + m + 'm';
    return 'expires in <1m';
  };
  const tick = () => {
    const now = Date.now() / 1000;
    document.querySelectorAll('.auth-expires[data-expires]').forEach(el => {
      const exp = Number(el.dataset.expires);
      // A zero/sentinel expiry (year-1 epoch) means "unknown" — don't show it as expired.
      if (!exp || exp < 1e9) { el.textContent = 'expires —'; el.classList.remove('auth-soon', 'auth-expired'); return; }
      const rem = exp - now;
      el.textContent = fmt(rem);
      el.classList.toggle('auth-soon', rem > 0 && rem < SOON);
      el.classList.toggle('auth-expired', rem <= 0);
    });
  };
  tick();
  setInterval(tick, 1000);
})();

// Chat user-profile popover. Each chat username (.cu) carries hx-get, so htmx
// fetches /admin/user/<name> into #user-popover on click — including usernames
// on SSE-added lines, since htmx processes swapped-in content (no delegation
// needed). We only record where the click landed and, on htmx:afterSwap, reveal
// + float the card near it; clicking outside or Escape closes it.
(function() {
  const pop = document.getElementById('user-popover');
  if (!pop) return;
  let lastXY = null, open = false;
  document.addEventListener('click', (e) => {
    const cu = e.target.closest('.cu');
    if (cu) { lastXY = { x: e.clientX, y: e.clientY }; return; } // htmx issues the GET
    if (open && !pop.contains(e.target)) { pop.hidden = true; open = false; }
  });
  pop.addEventListener('htmx:afterSwap', () => {
    pop.hidden = false; // unhide before measuring so offset* are real
    open = true;
    const pad = 8, xy = lastXY || { x: pad, y: pad };
    const x = Math.min(xy.x, window.innerWidth - pop.offsetWidth - pad);
    const y = Math.min(xy.y + pad, window.innerHeight - pop.offsetHeight - pad);
    pop.style.left = Math.max(pad, x) + 'px';
    pop.style.top = Math.max(pad, y) + 'px';
  });
  document.addEventListener('keydown', (e) => { if (e.key === 'Escape') { pop.hidden = true; open = false; } });
})();

// Live location map (Leaflet). Seeds a breadcrumb polyline + 🚐 pin from the
// data-trail attribute, then extends it as "map" SSE points arrive — htmx swaps
// each into #map-sink and we read its data-lat/lng on afterSwap. Tiles load from
// OpenStreetMap. No-op if Leaflet didn't load.
(function() {
  const el = document.getElementById('map');
  if (!el || typeof L === 'undefined') return;
  let trail = [];
  try { trail = JSON.parse(el.dataset.trail || '[]'); } catch (e) { trail = []; }

  const map = L.map(el);
  L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
    maxZoom: 19, attribution: '© OpenStreetMap contributors',
  }).addTo(map);

  const van = L.divIcon({ className: 'van-marker', html: '🚐', iconSize: [24, 24], iconAnchor: [12, 12] });

  // Consecutive breadcrumbs farther apart than this are treated as a
  // discontinuity (a timewarp clip or a bad GPS fix), not a driven path: the
  // solid trail breaks and the gap is bridged with a faint dashed segment so the
  // jump reads as "not real driving" rather than a straight slash across the map.
  const JUMP_KM = 50;
  const distKm = (a, b) => {
    const R = 6371, rad = d => d * Math.PI / 180;
    const dLat = rad(b[0] - a[0]), dLng = rad(b[1] - a[1]);
    const s = Math.sin(dLat / 2) ** 2 + Math.cos(rad(a[0])) * Math.cos(rad(b[0])) * Math.sin(dLng / 2) ** 2;
    return 2 * R * Math.asin(Math.sqrt(s));
  };
  // Split the trail into solid runs (consecutive points within JUMP_KM) and the
  // dashed bridge segments spanning each jump. Both are fed to Leaflet as
  // multi-polylines (arrays of latlng arrays).
  const splitTrail = (pts) => {
    const runs = [], bridges = [];
    if (!pts.length) return { runs, bridges };
    let run = [pts[0]];
    for (let i = 1; i < pts.length; i++) {
      if (distKm(pts[i - 1], pts[i]) > JUMP_KM) {
        runs.push(run);
        bridges.push([pts[i - 1], pts[i]]);
        run = [pts[i]];
      } else {
        run.push(pts[i]);
      }
    }
    runs.push(run);
    return { runs, bridges };
  };

  const solid = L.polyline([], { color: '#58a6ff', weight: 3, opacity: 0.75 }).addTo(map);
  const dashed = L.polyline([], { color: '#58a6ff', weight: 2, opacity: 0.4, dashArray: '4 6' }).addTo(map);
  const drawTrail = () => {
    const { runs, bridges } = splitTrail(trail);
    solid.setLatLngs(runs);
    dashed.setLatLngs(bridges);
  };
  drawTrail();
  let marker = null;
  let hasPoint = false;

  const recenter = (animate) => {
    const p = trail[trail.length - 1];
    if (!p) { map.setView([39.5, -98.35], 4); return; } // continental US until a fix arrives
    if (marker) marker.setLatLng(p); else marker = L.marker(p, { icon: van }).addTo(map);
    if (!hasPoint) { map.setView(p, 7); hasPoint = true; } // zoom in on the first point
    else { map.panTo(p, { animate: !!animate }); }         // later: follow, keep the user's zoom
  };
  recenter(false);

  const sink = document.getElementById('map-sink');
  if (sink) sink.addEventListener('htmx:afterSwap', () => {
    const span = sink.querySelector('span[data-lat]');
    if (!span) return;
    const lat = parseFloat(span.dataset.lat), lng = parseFloat(span.dataset.lng);
    if (isNaN(lat) || isNaN(lng)) return;
    trail.push([lat, lng]);
    if (trail.length > 100) trail.shift();
    drawTrail();
    recenter(true);
  });

  // "show full route" toggle: lazily fetch the whole-corpus route and draw it as
  // a faint background line behind the live trail. Remembered in localStorage.
  const corpusBtn = document.getElementById('corpus-toggle');
  let corpusLine = null;
  const renderCorpus = (fit) => {
    fetch('/admin/map/corpus').then(r => r.ok ? r.json() : Promise.reject(r.status)).then(pts => {
      corpusLine = L.polyline(pts, { color: '#888', weight: 1.5, opacity: 0.4 }).addTo(map);
      corpusLine.bringToBack();
      if (fit && pts.length) map.fitBounds(corpusLine.getBounds(), { padding: [20, 20] });
    }).catch(() => {});
  };
  const setCorpus = (on, fit) => {
    if (on) {
      if (corpusLine) { corpusLine.addTo(map); if (fit) map.fitBounds(corpusLine.getBounds(), { padding: [20, 20] }); }
      else renderCorpus(fit);
    } else if (corpusLine) {
      map.removeLayer(corpusLine);
    }
    if (corpusBtn) corpusBtn.textContent = on ? 'hide full route' : 'show full route';
    localStorage.setItem('map-corpus', on ? '1' : '0');
  };
  if (corpusBtn) corpusBtn.addEventListener('click', () => setCorpus(!(corpusLine && map.hasLayer(corpusLine)), true));
  setCorpus(localStorage.getItem('map-corpus') === '1', false);

  // The map sits in a <details>; Leaflet needs a size recalc when the container
  // is first laid out or revealed.
  const fix = () => map.invalidateSize();
  const details = el.closest('details');
  if (details) details.addEventListener('toggle', () => { if (details.open) setTimeout(fix, 50); });
  setTimeout(fix, 100);
})();

// No focus/visibility reload here anymore. The shell is cached for an instant
// repaint and the panel is live (SSE + the 15s /admin/refresh poll), so
// returning to the tab restores the last view (via the back/forward cache) and
// self-updates within seconds — a hard reload would just reintroduce the white
// flash on every return that we're trying to avoid.

// Molly switch, integrated with htmx. Action buttons (restart / stream toggle)
// carry data-arm-label: the first click arms the button (relabel + redden, 5s
// window) and cancels the htmx request; a second click within the window lets it
// through. Click-away or timeout disarms. htmx:confirm fires before every
// request, so we gate the molly buttons there and let everything else pass.
(function() {
  let armed = null, timer = null;
  const disarm = () => {
    if (!armed) return;
    armed.dataset.armed = '';
    armed.classList.remove('armed');
    armed.textContent = armed.dataset.originalLabel;
    armed = null;
    clearTimeout(timer);
    document.removeEventListener('click', outside, true);
  };
  const outside = (e) => { if (armed && e.target !== armed) disarm(); };
  document.body.addEventListener('htmx:confirm', (e) => {
    const btn = e.detail.elt;
    if (!btn || !btn.dataset || !btn.dataset.armLabel) return; // not a molly button
    if (btn.dataset.armed === '1') { disarm(); return; }       // armed → let it fire
    e.preventDefault();                                        // first click: arm + cancel
    btn.dataset.armed = '1';
    btn.classList.add('armed');
    btn.textContent = btn.dataset.armLabel;
    armed = btn;
    timer = setTimeout(disarm, 5000);
    // capture-phase listener so clicks anywhere else disarm before they bubble
    setTimeout(() => document.addEventListener('click', outside, true), 0);
  });
})();

// Stream preview follows a live toggle. obsStreamActionHandler returns an
// HX-Trigger: tripbot:stream-changed event carrying the new state; open or close
// the preview disclosure to match, so a toggle made from this panel updates the
// preview without a reload. Setting .open fires the disclosure's own toggle
// listener, which lazy-loads / unloads the iframe.
(function() {
  const details = document.getElementById('stream-preview');
  if (!details) return;
  document.body.addEventListener('tripbot:stream-changed', (e) => {
    const active = !!(e.detail && e.detail.active);
    if (details.open !== active) details.open = active;
  });
})();
</script>
</body>
</html>`))
