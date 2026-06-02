package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"strings"
	"sync"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/eventbus"
	"github.com/adanalife/tripbot/pkg/natsclient"
	"github.com/nats-io/nats.go"
)

// chatRingSize bounds the in-memory recent-chat history the panel renders on
// load. NATS core has no replay, so "recent history" is whatever has
// accumulated since the hub subscribed — for a process that runs for days
// that's effectively the whole recent stream. Doubles as the panel's scrollback
// depth (the browser DOM cap matches this).
const chatRingSize = 500

// sseClientBuffer is the per-client channel depth. A browser that can't keep up
// drops events (see broadcast) rather than stalling the NATS callback.
const sseClientBuffer = 64

// mapTrailSize bounds the breadcrumb trail behind the current-location pin —
// the last N GPS points the panel renders on load and extends live. ~100 video
// changes is a few hours of route.
const mapTrailSize = 100

// mapPoint is one GPS breadcrumb (decimal degrees).
type mapPoint struct {
	Lat float64
	Lng float64
}

// ChatLine is one rendered chat row held in the ring buffer. At is the message
// timestamp (UTC, from the event's emitted_at); the panel localizes it for
// display.
type ChatLine struct {
	Username string
	Text     string
	At       time.Time
}

// sseEvent is one named Server-Sent Event: Name routes it to an HTMX sse-swap
// target, Data is the pre-rendered (and HTML-escaped) one-line fragment.
type sseEvent struct {
	Name string
	Data string
}

// Hub is the live-console fan-out: it subscribes to eventbus observation events
// on NATS, keeps a bounded recent-chat ring, and broadcasts rendered fragments
// to every connected SSE client. One per process (eventHub).
//
// Detach note: the hub depends only on NATS for its events, so the admin panel
// could be lifted into its own service later (see plan) — nothing here reaches
// into pkg/chatbot or pkg/video in-process.
type Hub struct {
	mu   sync.RWMutex
	chat []ChatLine
	subs map[chan sseEvent]struct{}

	// viewers is the last chatter total seen; viewersKnown gates the very first
	// value so it sets the baseline without flashing (we can't know a direction
	// with nothing to compare against).
	viewers      int
	viewersKnown bool

	// mapTrail is the recent GPS breadcrumb trail (bounded by mapTrailSize),
	// appended on each video.changed that carries a real fix.
	mapTrail []mapPoint

	// nowPlaying is the last video.changed seen, cached so the initial page
	// render can show "now playing" from NATS instead of reaching into
	// pkg/video in-process. nowPlayingKnown gates it (nothing seen yet → the
	// panel hides the card). Populated by handleVideoChanged.
	nowPlaying      eventbus.VideoChanged
	nowPlayingKnown bool
}

// NewHub returns an unstarted hub. Safe to construct at package-init time — it
// touches neither NATS nor the network until Start.
func NewHub() *Hub {
	return &Hub{subs: make(map[chan sseEvent]struct{})}
}

// Start subscribes to the env's eventbus subjects and arranges teardown on
// ctx.Done. No-op (logs + returns) when NATS is unconfigured, so local dev and
// tests run fine with an empty console.
func (h *Hub) Start(ctx context.Context) {
	// Auth state is pull-based (not over NATS), so the countdown card works even
	// when NATS is unconfigured — start it before the nil-conn bail-out.
	go h.pollAuth(ctx)

	conn := natsclient.Conn()
	if conn == nil {
		slog.InfoContext(ctx, "live-console hub: NATS unconfigured; console will be empty")
		return
	}

	env := c.Conf.Environment
	subscriptions := []struct {
		subject string
		handler func(context.Context, []byte)
	}{
		{eventbus.ChatMessageSubject(env), h.handleChat},
		{eventbus.ViewerCountSubject(env), h.handleViewerCount},
		{eventbus.VideoChangedSubject(env), h.handleVideoChanged},
	}

	var subs []*nats.Subscription
	for _, s := range subscriptions {
		handler := s.handler // capture per-iteration for the closure
		sub, err := conn.Subscribe(s.subject, func(m *nats.Msg) {
			handler(ctx, m.Data)
		})
		if err != nil {
			slog.ErrorContext(ctx, "live-console hub subscribe failed", "err", err, "subject", s.subject)
			continue
		}
		slog.InfoContext(ctx, "live-console hub subscribed", "subject", s.subject)
		subs = append(subs, sub)
	}

	go func() {
		<-ctx.Done()
		for _, sub := range subs {
			_ = sub.Unsubscribe()
		}
		h.closeAll()
	}()
}

func (h *Hub) handleChat(ctx context.Context, data []byte) {
	var ev eventbus.ChatMessage
	if err := json.Unmarshal(data, &ev); err != nil {
		slog.ErrorContext(ctx, "live-console hub: bad chat payload", "err", err)
		return
	}
	line := ChatLine{Username: ev.Username, Text: ev.Text, At: parseEmitted(ev.EmittedAt)}
	h.appendChat(line)
	h.broadcast(sseEvent{Name: "chat", Data: renderChatLine(line)})
}

// handleViewerCount updates the live "in chat" number. It compares the new
// total to the last one seen so the panel can flash the count green (rising)
// or red (falling); the first value seen just sets the baseline (no flash).
func (h *Hub) handleViewerCount(ctx context.Context, data []byte) {
	var ev eventbus.ViewerCount
	if err := json.Unmarshal(data, &ev); err != nil {
		slog.ErrorContext(ctx, "live-console hub: bad viewer-count payload", "err", err)
		return
	}
	dir := h.updateViewers(ev.Count)
	h.broadcast(sseEvent{Name: "viewers", Data: renderViewerCount(ev.Count, dir)})
}

// updateViewers records the new count and returns the flash direction relative
// to the previous value: "up", "down", or "" (unchanged, or the first value).
func (h *Hub) updateViewers(count int) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	dir := ""
	if h.viewersKnown {
		switch {
		case count > h.viewers:
			dir = "up"
		case count < h.viewers:
			dir = "down"
		}
	}
	h.viewers = count
	h.viewersKnown = true
	return dir
}

// handleVideoChanged caches the switch as the current "now playing" (so the
// initial page render reads it from here instead of pkg/video in-process) and
// forwards it to connected panels' "now playing" card.
func (h *Hub) handleVideoChanged(ctx context.Context, data []byte) {
	var ev eventbus.VideoChanged
	if err := json.Unmarshal(data, &ev); err != nil {
		slog.ErrorContext(ctx, "live-console hub: bad video payload", "err", err)
		return
	}
	h.setNowPlaying(ev)
	h.broadcast(sseEvent{Name: "video", Data: renderVideoLine(ev)})

	// Drop a breadcrumb for the live map when the clip has a real GPS fix.
	if hasFix(ev) {
		p := mapPoint{Lat: ev.Lat, Lng: ev.Lng}
		h.appendMapPoint(p)
		h.broadcast(sseEvent{Name: "map", Data: renderMapPoint(p)})
	}
}

// setNowPlaying caches the latest video.changed as the current clip.
func (h *Hub) setNowPlaying(ev eventbus.VideoChanged) {
	h.mu.Lock()
	h.nowPlaying = ev
	h.nowPlayingKnown = true
	h.mu.Unlock()
}

// snapshotNowPlaying returns the last video.changed seen and whether one has
// arrived yet, for the initial page render.
func (h *Hub) snapshotNowPlaying() (eventbus.VideoChanged, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.nowPlaying, h.nowPlayingKnown
}

// hasFix reports whether a video.changed carries usable coordinates — flagged
// clips have no GPS, and 0/0 (null island) is treated as missing.
func hasFix(ev eventbus.VideoChanged) bool {
	return !ev.Flagged && (ev.Lat != 0 || ev.Lng != 0)
}

func (h *Hub) appendMapPoint(p mapPoint) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.mapTrail = append(h.mapTrail, p)
	if len(h.mapTrail) > mapTrailSize {
		h.mapTrail = append([]mapPoint(nil), h.mapTrail[len(h.mapTrail)-mapTrailSize:]...)
	}
}

// snapshotMapTrail copies the breadcrumb trail (oldest first) for the initial
// page render.
func (h *Hub) snapshotMapTrail() []mapPoint {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]mapPoint, len(h.mapTrail))
	copy(out, h.mapTrail)
	return out
}

// renderMapPoint is the SSE "map" fragment swapped into #map-sink; the map JS
// reads data-lat/data-lng off it on htmx:afterSwap and drops a breadcrumb. 6
// decimals is ~0.1m — plenty, and avoids %g scientific notation.
func renderMapPoint(p mapPoint) string {
	return fmt.Sprintf(`<span data-lat="%.6f" data-lng="%.6f"></span>`, p.Lat, p.Lng)
}

// mapTrailJSON returns the server hub's breadcrumb trail as a JSON
// [[lat,lng],…] string for the page's map data attribute (empty "[]" when
// there's no trail yet).
func (s *Server) mapTrailJSON() string {
	trail := s.hub.snapshotMapTrail()
	pts := make([][2]float64, len(trail))
	for i, p := range trail {
		pts[i] = [2]float64{p.Lat, p.Lng}
	}
	b, err := json.Marshal(pts)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// parseEmitted turns an event's emitted_at (RFC3339Nano UTC) into a time,
// falling back to now so a malformed/empty value still gets a sensible stamp.
func parseEmitted(s string) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	return time.Now().UTC()
}

func (h *Hub) appendChat(line ChatLine) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.chat = append(h.chat, line)
	if len(h.chat) > chatRingSize {
		// Drop oldest; reallocate (cap 0) so the backing array stays bounded.
		h.chat = append([]ChatLine(nil), h.chat[len(h.chat)-chatRingSize:]...)
	}
}

// snapshotChat returns a copy of the recent-chat ring, oldest first, for the
// initial page render.
func (h *Hub) snapshotChat() []ChatLine {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]ChatLine, len(h.chat))
	copy(out, h.chat)
	return out
}

// register adds an SSE client and returns its event channel.
func (h *Hub) register() chan sseEvent {
	ch := make(chan sseEvent, sseClientBuffer)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// unregister removes a client and closes its channel. Idempotent — safe to call
// from the handler's defer even after closeAll already removed it.
func (h *Hub) unregister(ch chan sseEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
}

// broadcast sends ev to every client without blocking: a full client buffer
// (slow browser) drops the event rather than stalling the NATS callback. The
// RWMutex makes this mutually exclusive with unregister/closeAll, so we never
// send on a closed channel.
func (h *Hub) broadcast(ev sseEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

// numSubscribers reports the count of connected SSE clients (used by tests).
func (h *Hub) numSubscribers() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs)
}

func (h *Hub) closeAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		delete(h.subs, ch)
		close(ch)
	}
}

// chatLineTmpl renders one chat row as a single line (no newlines — SSE data
// must not contain bare newlines). html/template escapes username + text.
var chatLineTmpl = template.Must(template.New("chatline").Parse(
	`<div class="chat-line"><time class="ct-ts" datetime="{{.At.Format "2006-01-02T15:04:05Z07:00"}}">{{.At.Format "15:04"}}</time> <span class="cu" hx-get="/admin/user/{{.Username}}" hx-target="#user-popover" hx-swap="innerHTML" hx-trigger="click">{{.Username}}</span> <span class="ct">{{.Text}}</span></div>`))

func renderChatLine(line ChatLine) string {
	var sb strings.Builder
	if err := chatLineTmpl.Execute(&sb, line); err != nil {
		slog.Error("live-console hub: render chat line", "err", err)
		return ""
	}
	return sb.String()
}

// viewerCountTmpl renders the inner chatter-count span. It's swapped into the
// stable #chatters container (innerHTML), so a fresh element is inserted each
// update — the flash-up/flash-down CSS animation re-triggers naturally on
// insert, and the steady state (no flash class) has no animation.
var viewerCountTmpl = template.Must(template.New("viewers").Parse(
	`<span class="chatters-count{{if .Flash}} flash-{{.Flash}}{{end}}">{{.Count}}</span>`))

func renderViewerCount(count int, dir string) string {
	var sb strings.Builder
	if err := viewerCountTmpl.Execute(&sb, struct {
		Count int
		Flash string
	}{Count: count, Flash: dir}); err != nil {
		slog.Error("live-console hub: render viewer count", "err", err)
		return ""
	}
	return sb.String()
}

// videoLineTmpl renders the inner of the "now playing" line — file, state, and
// an elapsed-time span the page's JS ticker counts up from data-since. Swapped
// (innerHTML) into the stable #now-line target. The markup mirrors the
// server-rendered version in admin.go's template (same duplication the chat
// line uses) so a live swap looks identical to a fresh page render. A
// just-switched clip starts at 0s; the ticker takes over within a second.
var videoLineTmpl = template.Must(template.New("videoline").Parse(
	`<span class="file">{{.File}}</span>{{if .State}} <span class="state">· {{.State}}</span>{{end}} <span class="state">· <span class="now-elapsed" data-since="{{.SinceUnix}}">{{.Progress}}</span></span>`))

func renderVideoLine(ev eventbus.VideoChanged) string {
	var sb strings.Builder
	if err := videoLineTmpl.Execute(&sb, struct {
		File, State, Progress string
		SinceUnix             int64
	}{
		File:      ev.File,
		State:     ev.State,
		Progress:  "0s",
		SinceUnix: parseEmitted(ev.EmittedAt).Unix(),
	}); err != nil {
		slog.Error("live-console hub: render video line", "err", err)
		return ""
	}
	return sb.String()
}

// Server.hub is the process-wide live-console hub. Constructed in New()
// (cheap, no I/O); its NATS subscription starts later via StartEventHub.

// StartEventHub begins the hub's NATS subscription. Call from main() AFTER
// natsclient.Connect — at server.Start time NATS isn't connected yet.
func (s *Server) StartEventHub(ctx context.Context) { s.hub.Start(ctx) }
