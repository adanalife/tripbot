// Package eventbus publishes tripbot *observation* events to NATS — facts that
// "something happened" (a chat line arrived, the video changed, a viewer
// joined) for any subscriber to observe. The admin panel's live console is the
// first consumer. Every emit is fire-and-forget; when NATS is unconfigured
// (natsclient.Conn() is nil) each emit is a silent no-op.
//
// This is deliberately distinct from two neighbours:
//   - pkg/events — the Postgres-backed append-only session log (login/logout
//     rows). That's durable state; this is ephemeral pub/sub.
//   - onscreens *commands* (ShowMiddleText, ShowFlag, …) — imperatives aimed at
//     onscreens-server, with exactly one consumer that acts on them. Those live
//     with the onscreens surface, not here.
//
// Subjects follow the project convention tripbot.<env>.<domain>.<event>;
// envelopes are snake_case JSON carrying emitted_at (RFC3339Nano UTC) so a
// future protobuf schema maps 1-1. See the NATS phase-0/1 session notes.
package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/adanalife/tripbot/pkg/natsclient"
	"github.com/nats-io/nats.go/jetstream"
)

// Publisher is the fire-and-forget publish surface the Emit helpers use. Tests
// inject a fake via SetPublisher; production uses realPublisher, which delegates
// to the pkg/natsclient singleton.
type Publisher interface {
	Publish(ctx context.Context, subject string, payload []byte)
}

// realPublisher reads the natsclient singleton lazily on each call so a NATS
// connection that lands after package init (always — main runs after package
// vars) is picked up. Errors are logged, never returned.
type realPublisher struct{}

func (realPublisher) Publish(ctx context.Context, subject string, payload []byte) {
	conn := natsclient.Conn()
	if conn == nil {
		return
	}
	if err := conn.Publish(subject, payload); err != nil {
		slog.ErrorContext(ctx, "eventbus publish failed", "err", err, "subject", subject)
	}
}

// Default is the package publisher the Emit helpers send through. Overridable in
// tests via SetPublisher (mirrors natsclient.SetConn).
var Default Publisher = realPublisher{}

// SetPublisher swaps the package publisher. Tests pass a recording fake; pass
// realPublisher{} to restore.
func SetPublisher(p Publisher) { Default = p }

// nowFn is overridable in tests for a deterministic emitted_at.
var nowFn = func() time.Time { return time.Now().UTC() }

func emittedAt() string { return nowFn().Format(time.RFC3339Nano) }

// subject builds the canonical tripbot.<env>.<domain>.<event> NATS subject.
func subject(env, domain, event string) string {
	return fmt.Sprintf("tripbot.%s.%s.%s", env, domain, event)
}

func emit(ctx context.Context, subj string, ev any) {
	payload, err := json.Marshal(ev)
	if err != nil {
		slog.ErrorContext(ctx, "eventbus marshal failed", "err", err, "subject", subj)
		return
	}
	Default.Publish(ctx, subj, payload)
}

// --- chat.message ---------------------------------------------------------

// ChatMessage is the wire format for tripbot.<env>.chat.message — one incoming
// Twitch chat line.
type ChatMessage struct {
	Username  string `json:"username"`
	Text      string `json:"text"`
	EmittedAt string `json:"emitted_at"`
}

// ChatMessageSubject returns the subscribe/publish subject for chat messages in
// env. Subscribers (the admin hub) build the same string to subscribe.
func ChatMessageSubject(env string) string { return subject(env, "chat", "message") }

// EmitChatMessage publishes an incoming chat line. Pass the original-case
// username + text (not the lowercased command-parse copy).
func EmitChatMessage(ctx context.Context, env, username, text string) {
	emit(ctx, ChatMessageSubject(env), ChatMessage{
		Username:  username,
		Text:      text,
		EmittedAt: emittedAt(),
	})
}

// --- viewers.count --------------------------------------------------------

// ViewerCount is the wire format for tripbot.<env>.viewers.count — the
// authoritative chatter total Twitch reports, published on each chatter-list
// refresh so the admin panel's "in chat" number updates live.
type ViewerCount struct {
	Count     int    `json:"count"`
	EmittedAt string `json:"emitted_at"`
}

// ViewerCountSubject returns the subscribe/publish subject for viewer-count
// updates in env. The admin hub builds the same string to subscribe.
func ViewerCountSubject(env string) string { return subject(env, "viewers", "count") }

// EmitViewerCount publishes the current chatter total. The admin hub compares
// it to the previous value to flash the count green (rising) or red (falling).
func EmitViewerCount(ctx context.Context, env string, count int) {
	emit(ctx, ViewerCountSubject(env), ViewerCount{
		Count:     count,
		EmittedAt: emittedAt(),
	})
}

// --- video.changed --------------------------------------------------------

// VideoChanged is the wire format for tripbot.<env>.video.changed — published
// when VLC switches to a new clip. State is the full state name (e.g.
// "Wyoming"); Flagged marks a no-GPS clip. The admin panel's "now playing"
// card updates from this without a reload.
type VideoChanged struct {
	File      string  `json:"file"`
	State     string  `json:"state"`
	Flagged   bool    `json:"flagged"`
	Lat       float64 `json:"lat"` // GPS of the clip; 0/0 + Flagged means no fix
	Lng       float64 `json:"lng"`
	EmittedAt string  `json:"emitted_at"`
}

// VideoChangedSubject returns the subscribe/publish subject for video-change
// events in env.
func VideoChangedSubject(env string) string { return subject(env, "video", "changed") }

// EmitVideoChanged publishes a video switch. The emitted_at doubles as the
// clip's start time, so the panel can tick an elapsed timer from it.
func EmitVideoChanged(ctx context.Context, env, file, state string, flagged bool, lat, lng float64) {
	emit(ctx, VideoChangedSubject(env), VideoChanged{
		File:      file,
		State:     state,
		Flagged:   flagged,
		Lat:       lat,
		Lng:       lng,
		EmittedAt: emittedAt(),
	})
}

// --- JetStream streams (durable history) ----------------------------------
//
// Two subjects need to survive a tripbot reboot so the admin live console can
// backfill on load: chat.message (the chat log) and video.changed (the
// now-playing card + the live-map breadcrumb trail). Each gets its own stream
// so its retention cap is independent — a stream's MaxMsgs is whole-stream, and
// these two want different depths. Publishers are unchanged: a core publish to a
// subject a stream covers is captured automatically. viewers.count is NOT
// streamed — it's a momentary value with nothing to replay.

const (
	chatStreamName  = "TRIPBOT_CHAT"
	videoStreamName = "TRIPBOT_VIDEO"
)

// Retention caps match the admin hub's in-memory buffer sizes (pkg/server:
// chatRingSize=500, mapTrailSize=100) so a startup backfill exactly refills
// them. Video keeps headroom over the 100-point trail since each video.changed
// is one clip, not one breadcrumb (flagged/no-fix clips drop no breadcrumb).
const (
	chatStreamMaxMsgs  = 500
	videoStreamMaxMsgs = 200
)

// EnsureStreams idempotently declares the JetStream streams backing the admin
// live console's durable buffers. Both are file-backed and bounded to the most
// recent N messages (DiscardOld), so the hub replays recent history after a
// restart and JetStream evicts the oldest beyond the cap.
//
// No-op when js is nil (NATS off / JetStream unavailable) — the hub then falls
// back to live-only core subscriptions. Safe on every startup:
// CreateOrUpdateStream reconciles config in place, so changing a cap later just
// updates the stream. Stream names are constant: each env runs its own NATS in
// its own namespace, so there's no cross-env collision.
func EnsureStreams(ctx context.Context, js jetstream.JetStream, env string) error {
	if js == nil {
		return nil
	}
	configs := []jetstream.StreamConfig{
		{
			Name:        chatStreamName,
			Description: "Admin live-console chat history (bounded recent ring).",
			Subjects:    []string{ChatMessageSubject(env)},
			Storage:     jetstream.FileStorage,
			Retention:   jetstream.LimitsPolicy,
			Discard:     jetstream.DiscardOld,
			MaxMsgs:     chatStreamMaxMsgs,
		},
		{
			Name:        videoStreamName,
			Description: "Admin live-console video.changed history (now-playing + map trail).",
			Subjects:    []string{VideoChangedSubject(env)},
			Storage:     jetstream.FileStorage,
			Retention:   jetstream.LimitsPolicy,
			Discard:     jetstream.DiscardOld,
			MaxMsgs:     videoStreamMaxMsgs,
		},
	}
	for _, cfg := range configs {
		if _, err := js.CreateOrUpdateStream(ctx, cfg); err != nil {
			return fmt.Errorf("ensure stream %s: %w", cfg.Name, err)
		}
	}
	slog.InfoContext(ctx, "jetstream streams ensured", "streams", chatStreamName+","+videoStreamName, "env", env)
	return nil
}

// ChatStreamName and VideoStreamName expose the stream names so consumers (the
// admin hub) can bind ordered consumers to them without re-deriving the
// constants.
func ChatStreamName() string  { return chatStreamName }
func VideoStreamName() string { return videoStreamName }
