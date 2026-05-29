package server

import (
	"context"
	"encoding/json"
	"html/template"
	"log/slog"
	"strings"
	"sync"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/eventbus"
	"github.com/adanalife/tripbot/pkg/natsclient"
	"github.com/nats-io/nats.go"
)

// chatRingSize bounds the in-memory recent-chat history the panel renders on
// load. NATS core has no replay, so "recent history" is whatever has
// accumulated since the hub subscribed — for a process that runs for days
// that's effectively the whole recent stream.
const chatRingSize = 200

// sseClientBuffer is the per-client channel depth. A browser that can't keep up
// drops events (see broadcast) rather than stalling the NATS callback.
const sseClientBuffer = 64

// ChatLine is one rendered chat row held in the ring buffer.
type ChatLine struct {
	Username string
	Text     string
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
	conn := natsclient.Conn()
	if conn == nil {
		slog.InfoContext(ctx, "live-console hub: NATS unconfigured; console will be empty")
		return
	}

	subj := eventbus.ChatMessageSubject(c.Conf.Environment)
	sub, err := conn.Subscribe(subj, func(m *nats.Msg) {
		h.handleChat(ctx, m.Data)
	})
	if err != nil {
		slog.ErrorContext(ctx, "live-console hub subscribe failed", "err", err, "subject", subj)
		return
	}
	slog.InfoContext(ctx, "live-console hub subscribed", "subject", subj)

	go func() {
		<-ctx.Done()
		_ = sub.Unsubscribe()
		h.closeAll()
	}()
}

func (h *Hub) handleChat(ctx context.Context, data []byte) {
	var ev eventbus.ChatMessage
	if err := json.Unmarshal(data, &ev); err != nil {
		slog.ErrorContext(ctx, "live-console hub: bad chat payload", "err", err)
		return
	}
	line := ChatLine{Username: ev.Username, Text: ev.Text}
	h.appendChat(line)
	h.broadcast(sseEvent{Name: "chat", Data: renderChatLine(line)})
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
	`<div class="chat-line"><span class="cu">{{.Username}}</span> <span class="ct">{{.Text}}</span></div>`))

func renderChatLine(line ChatLine) string {
	var sb strings.Builder
	if err := chatLineTmpl.Execute(&sb, line); err != nil {
		slog.Error("live-console hub: render chat line", "err", err)
		return ""
	}
	return sb.String()
}

// eventHub is the process-wide live-console hub. Constructed at package init
// (cheap, no I/O); its NATS subscription starts later via StartEventHub.
var eventHub = NewHub()

// StartEventHub begins the hub's NATS subscription. Call from main() AFTER
// natsclient.Connect — at server.Start time NATS isn't connected yet.
func StartEventHub(ctx context.Context) { eventHub.Start(ctx) }
