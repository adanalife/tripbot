// Package eventbus publishes tripbot *observation* events to NATS — facts that
// "something happened" (a chat line arrived, the video changed, a viewer
// joined) for any subscriber to observe. The admin panel's live console is the
// first consumer. Every emit is fire-and-forget; when NATS is unconfigured
// (natsclient.Conn() is nil) each emit is a silent no-op.
//
// This is deliberately distinct from two neighbours:
//   - pkg/events — the Postgres-backed append-only session log (login/logout
//     rows). That's durable state; this is ephemeral pub/sub.
//   - onscreens *commands* (ShowMiddleText, ShowTimewarp, …) — imperatives aimed at
//     onscreens-server, with exactly one consumer that acts on them. Those live
//     with the onscreens surface, not here.
//
// Subjects follow the project convention tripbot.<env>.<domain>.<event>;
// envelopes are snake_case JSON carrying emitted_at (RFC3339Nano UTC) so a
// future protobuf schema maps 1-1.
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
//
// ponytail: Publisher duplicates natsclient.Publisher (identical signature) and
// realPublisher re-implements natsclient's connPublisher. Could collapse onto
// natsclient.Publisher + natsclient.DefaultPublisher(). Kept as an explicit local
// seam for now — deferred 2026-06-29 (ponytail-audit).
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
// chat line from whichever platform this instance serves.
type ChatMessage struct {
	// Platform is the streaming platform the line came from ("twitch" /
	// "youtube"). Both per-platform instances publish into the same env's
	// subject, so this is what lets the admin console disambiguate. Empty on
	// events emitted before the tag existed.
	Platform  string `json:"platform,omitempty"`
	Username  string `json:"username"`
	Text      string `json:"text"`
	EmittedAt string `json:"emitted_at"`
}

// ChatMessageSubject returns the subscribe/publish subject for chat messages in
// env. Subscribers (the admin hub) build the same string to subscribe.
func ChatMessageSubject(env string) string { return subject(env, "chat", "message") }

// EmitChatMessage publishes an incoming chat line. Pass the original-case
// username + text (not the lowercased command-parse copy).
func EmitChatMessage(ctx context.Context, env, platform, username, text string) {
	emit(ctx, ChatMessageSubject(env), ChatMessage{
		Platform:  platform,
		Username:  username,
		Text:      text,
		EmittedAt: emittedAt(),
	})
}

// --- viewers.count --------------------------------------------------------

// ViewerCount is the wire format for tripbot.<env>.viewers.count — the
// authoritative chatter total the platform reports, published on each
// chatter-list refresh so the console's "in chat" number updates live.
type ViewerCount struct {
	// Platform is the streaming platform this count is for ("twitch" /
	// "youtube"). Both per-platform instances publish into the same env's
	// subject, so this is what lets the console keep a separate count per
	// platform instead of the instances clobbering one another. Empty on
	// events emitted before the tag existed.
	Platform  string `json:"platform,omitempty"`
	Count     int    `json:"count"`
	EmittedAt string `json:"emitted_at"`
}

// ViewerCountSubject returns the subscribe/publish subject for viewer-count
// updates in env. The console builds the same string to subscribe.
func ViewerCountSubject(env string) string { return subject(env, "viewers", "count") }

// EmitViewerCount publishes the current chatter total for this instance's
// platform. The console compares it to the previous value to flash the count
// green (rising) or red (falling).
func EmitViewerCount(ctx context.Context, env, platform string, count int) {
	emit(ctx, ViewerCountSubject(env), ViewerCount{
		Platform:  platform,
		Count:     count,
		EmittedAt: emittedAt(),
	})
}

// --- video.changed --------------------------------------------------------

// VideoChanged is the wire format for tripbot.<env>.video.changed — published
// when playout switches to a new clip. State is the full state name (e.g.
// "Wyoming"); Flagged marks a no-GPS clip. The console's "now playing" card
// updates from this without a reload.
type VideoChanged struct {
	// Platform is the streaming platform whose playout switched clips ("twitch" /
	// "youtube"). Each platform runs its own playout at an independent corpus
	// position, so both per-platform instances publish into the same env's
	// subject; this is what lets the console keep a separate now-playing card
	// and map trail per platform. Empty on events emitted before the tag
	// existed.
	Platform  string  `json:"platform,omitempty"`
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

// EmitVideoChanged publishes a video switch for this instance's platform. The
// emitted_at doubles as the clip's start time, so the console can tick an
// elapsed timer from it.
func EmitVideoChanged(ctx context.Context, env, platform, file, state string, flagged bool, lat, lng float64) {
	emit(ctx, VideoChangedSubject(env), VideoChanged{
		Platform:  platform,
		File:      file,
		State:     state,
		Flagged:   flagged,
		Lat:       lat,
		Lng:       lng,
		EmittedAt: emittedAt(),
	})
}

// --- auth.status ------------------------------------------------------------

// AuthAccount is one identity's token state inside an AuthStatus snapshot.
// Field semantics mirror mytwitch.AccountTokenStatus, but the type is defined
// here so the eventbus stays free of pkg/twitch (and its DB-reaching imports)
// per the package-boundary ADR — cmd/tripbot converts at the call site.
type AuthAccount struct {
	Account   string `json:"account"`              // "bot" | "broadcaster" | "youtube" — the consent account selector
	LoginAs   string `json:"login_as,omitempty"`   // the exact platform username/channel to sign in as
	ExpiresAt string `json:"expires_at,omitempty"` // RFC3339Nano UTC; empty when unknown (missing token, or auto-refreshed)
	Reason    string `json:"reason,omitempty"`     // "" healthy, else "missing" | "expired"
}

// AuthStatus is the wire format for tripbot.<env>.auth.status.<platform> — a
// full token-state snapshot for one platform instance's identities, emitted on
// a ~30s ticker. Consumers (the standalone console) render per-identity expiry
// countdowns from it; re-auth itself runs through the platform-gateway consent
// flow, so the console builds the re-auth link from the gateway host (mirroring
// how it already handles YouTube), not from this snapshot.
type AuthStatus struct {
	Platform  string        `json:"platform"`
	Accounts  []AuthAccount `json:"accounts"`
	EmittedAt string        `json:"emitted_at"`
}

// AuthStatusSubject returns the publish subject for one platform instance's
// auth snapshots. Unlike the other domains this subject is per-platform — the
// twitch and youtube instances own disjoint identities, and the per-platform
// leaf lets the TRIPBOT_AUTH stream keep a last-value cache of each
// (MaxMsgsPerSubject=1) instead of the instances clobbering one another.
func AuthStatusSubject(env, platform string) string {
	return subject(env, "auth", "status") + "." + platform
}

// AuthStatusWildcard returns the subscribe pattern covering every platform's
// auth snapshots in env.
func AuthStatusWildcard(env string) string { return subject(env, "auth", "status") + ".*" }

// EmitAuthStatus publishes a token-state snapshot for this instance's platform.
func EmitAuthStatus(ctx context.Context, env, platform string, accounts []AuthAccount) {
	emit(ctx, AuthStatusSubject(env, platform), AuthStatus{
		Platform:  platform,
		Accounts:  accounts,
		EmittedAt: emittedAt(),
	})
}

// --- youtube.broadcast ------------------------------------------------------

// YoutubeBroadcast is the wire format for tripbot.<env>.youtube.broadcast — the
// current live YouTube broadcast discovered via the gateway, emitted on a slow
// ticker by the youtube instance. VideoID is the watchable id
// (youtube.com/watch?v=<id>); Privacy is "public"/"unlisted"/"private". Live is
// false when no broadcast is active. The console needs this to link to (and
// embed) an unlisted broadcast, whose channel/handle "/live" redirect only
// resolves a public stream.
type YoutubeBroadcast struct {
	VideoID   string `json:"video_id"`
	Live      bool   `json:"live"`
	Privacy   string `json:"privacy"`
	EmittedAt string `json:"emitted_at"`
}

// YoutubeBroadcastSubject returns the subscribe/publish subject for the current
// YouTube broadcast in env. Only the youtube instance publishes it.
func YoutubeBroadcastSubject(env string) string { return subject(env, "youtube", "broadcast") }

// --- facebook.broadcast ------------------------------------------------------

// FacebookBroadcast is the wire format for tripbot.<env>.facebook.broadcast —
// the Page's current live video discovered via the gateway, emitted on a slow
// ticker by the facebook instance. VideoID is the watchable video object id
// (facebook.com/video.php?v=<id>); Privacy is "public"/"unpublished" —
// "unpublished" is the timeline-hidden (admin-only) dry-run state, facebook's
// analog of a youtube unlisted broadcast. Live is false when no broadcast is
// active. The console needs this to badge and link an unpublished rehearsal,
// which the Page timeline never shows.
type FacebookBroadcast struct {
	VideoID   string `json:"video_id"`
	Live      bool   `json:"live"`
	Privacy   string `json:"privacy"`
	EmittedAt string `json:"emitted_at"`
}

// FacebookBroadcastSubject returns the subscribe/publish subject for the
// current Facebook broadcast in env. Only the facebook instance publishes it.
func FacebookBroadcastSubject(env string) string { return subject(env, "facebook", "broadcast") }

// EmitFacebookBroadcast publishes the current Facebook broadcast snapshot. A
// last-value cache (TRIPBOT_FACEBOOK, MaxMsgsPerSubject=1) retains the latest
// so a fresh console renders the badge on connect.
func EmitFacebookBroadcast(ctx context.Context, env, videoID, privacy string, live bool) {
	emit(ctx, FacebookBroadcastSubject(env), FacebookBroadcast{
		VideoID:   videoID,
		Live:      live,
		Privacy:   privacy,
		EmittedAt: emittedAt(),
	})
}

// EmitYoutubeBroadcast publishes the current YouTube broadcast snapshot. A
// last-value cache (TRIPBOT_YOUTUBE, MaxMsgsPerSubject=1) retains the latest so a
// fresh console renders the link on connect.
func EmitYoutubeBroadcast(ctx context.Context, env, videoID, privacy string, live bool) {
	emit(ctx, YoutubeBroadcastSubject(env), YoutubeBroadcast{
		VideoID:   videoID,
		Live:      live,
		Privacy:   privacy,
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
	chatStreamName     = "TRIPBOT_CHAT"
	videoStreamName    = "TRIPBOT_VIDEO"
	authStreamName     = "TRIPBOT_AUTH"
	youtubeStreamName  = "TRIPBOT_YOUTUBE"
	facebookStreamName = "TRIPBOT_FACEBOOK"
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
		{
			Name:        authStreamName,
			Description: "Last-known auth.status snapshot per platform instance (last-value cache).",
			Subjects:    []string{AuthStatusWildcard(env)},
			Storage:     jetstream.FileStorage,
			Retention:   jetstream.LimitsPolicy,
			Discard:     jetstream.DiscardOld,
			// One retained message per subject leaf (= per platform): a fresh
			// console replays exactly the latest snapshot from each instance,
			// then live updates arrive on the same subscription.
			MaxMsgsPerSubject: 1,
		},
		{
			Name:        youtubeStreamName,
			Description: "Last-known YouTube broadcast snapshot (last-value cache).",
			Subjects:    []string{YoutubeBroadcastSubject(env)},
			Storage:     jetstream.FileStorage,
			Retention:   jetstream.LimitsPolicy,
			Discard:     jetstream.DiscardOld,
			// One retained message: a fresh console renders the current
			// watch/embed link on connect, then live updates follow.
			MaxMsgsPerSubject: 1,
		},
		{
			Name:        facebookStreamName,
			Description: "Last-known Facebook broadcast snapshot (last-value cache).",
			Subjects:    []string{FacebookBroadcastSubject(env)},
			Storage:     jetstream.FileStorage,
			Retention:   jetstream.LimitsPolicy,
			Discard:     jetstream.DiscardOld,
			// One retained message: a fresh console renders the current
			// badge/link on connect, then live updates follow.
			MaxMsgsPerSubject: 1,
		},
	}
	for _, cfg := range configs {
		if _, err := js.CreateOrUpdateStream(ctx, cfg); err != nil {
			return fmt.Errorf("ensure stream %s: %w", cfg.Name, err)
		}
	}
	slog.InfoContext(ctx, "jetstream streams ensured", "streams", chatStreamName+","+videoStreamName+","+authStreamName+","+youtubeStreamName+","+facebookStreamName, "env", env)
	return nil
}

// The *StreamName functions expose the stream names so consumers (the admin
// hub, the standalone console) can bind ordered consumers to them without
// re-deriving the constants.
func ChatStreamName() string              { return chatStreamName }
func VideoStreamName() string             { return videoStreamName }
func AuthStreamName() string              { return authStreamName }
func YoutubeBroadcastStreamName() string  { return youtubeStreamName }
func FacebookBroadcastStreamName() string { return facebookStreamName }
