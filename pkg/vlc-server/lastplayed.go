package vlcServer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	c "github.com/adanalife/tripbot/pkg/config/vlc-server"
	"github.com/adanalife/tripbot/pkg/natsclient"
	ve "github.com/adanalife/tripbot/pkg/vlc-events"
	"github.com/nats-io/nats.go/jetstream"
)

// lastPlayedStreamName is the JetStream stream holding each platform
// instance's most recent lastplayed publish — a last-value cache keyed by
// subject leaf (tripbot.<env>.vlc.lastplayed.<platform>), same shape as
// TRIPBOT_AUTH. A restarted vlc-server reads its own leaf back to resume the
// clip it was playing; instances never read each other's.
const lastPlayedStreamName = "TRIPBOT_VLC_LASTPLAYED"

// EnsureLastPlayedStream idempotently declares the lastplayed stream. Call it
// once at startup, before the first play — a core publish to a subject no
// stream covers is silently uncaptured, so the stream has to exist before
// playback starts for resume-on-restart to have anything to read. No-op when
// js is nil (NATS off / JetStream unavailable); resume then degrades to the
// watchdog marker + PlayRandom chain.
func EnsureLastPlayedStream(ctx context.Context, js jetstream.JetStream, env string) error {
	if js == nil {
		return nil
	}
	cfg := jetstream.StreamConfig{
		Name:        lastPlayedStreamName,
		Description: "Last-played video per platform vlc instance (last-value cache).",
		Subjects:    []string{ve.LastPlayedWildcard(env)},
		Storage:     jetstream.FileStorage,
		Retention:   jetstream.LimitsPolicy,
		Discard:     jetstream.DiscardOld,
		// One retained message per subject leaf (= per platform instance):
		// exactly the latest clip each instance started, nothing to prune.
		MaxMsgsPerSubject: 1,
	}
	if _, err := js.CreateOrUpdateStream(ctx, cfg); err != nil {
		return fmt.Errorf("ensure stream %s: %w", lastPlayedStreamName, err)
	}
	slog.InfoContext(ctx, "jetstream stream ensured", "stream", lastPlayedStreamName, "env", env)
	return nil
}

// publishLastPlayed publishes file as this instance's lastplayed state.
// Fire-and-forget core publish — the stream captures it; when NATS is
// unconfigured it's a silent no-op (matching the eventbus convention).
func publishLastPlayed(ctx context.Context, env, platform, file string) {
	conn := natsclient.Conn()
	if conn == nil {
		return
	}
	payload, err := json.Marshal(ve.LastPlayed{Envelope: ve.NewEnvelope(), File: file})
	if err != nil {
		slog.ErrorContext(ctx, "lastplayed marshal failed", "err", err, "file", file)
		return
	}
	subj := ve.LastPlayedSubject(env, platform)
	if err := conn.Publish(subj, payload); err != nil {
		slog.ErrorContext(ctx, "lastplayed publish failed", "err", err, "subject", subj)
	}
}

// announceLastPlayed is the config-bound wrapper playAtIndex calls on every
// successful play. Background ctx: the playback paths (NATS handlers, startup
// resume) don't carry a request context.
func (s *Server) announceLastPlayed(file string) {
	publishLastPlayed(context.Background(), c.Conf.Environment, c.Conf.Platform, file)
}

// lastPlayedFile reads the last-value cache back: the playlist basename this
// (env, platform) instance most recently published, or ok=false when there's
// nothing to resume (empty stream, NATS off, or any read error — resume is
// best-effort, so errors are logged and swallowed).
func lastPlayedFile(ctx context.Context, js jetstream.JetStream, env, platform string) (string, bool) {
	if js == nil {
		return "", false
	}
	stream, err := js.Stream(ctx, lastPlayedStreamName)
	if err != nil {
		slog.WarnContext(ctx, "lastplayed stream lookup failed", "err", err, "stream", lastPlayedStreamName)
		return "", false
	}
	subj := ve.LastPlayedSubject(env, platform)
	raw, err := stream.GetLastMsgForSubject(ctx, subj)
	if err != nil {
		if !errors.Is(err, jetstream.ErrMsgNotFound) {
			slog.WarnContext(ctx, "lastplayed read failed", "err", err, "subject", subj)
		}
		return "", false
	}
	var ev ve.LastPlayed
	if err := json.Unmarshal(raw.Data, &ev); err != nil {
		slog.WarnContext(ctx, "lastplayed decode failed", "err", err, "subject", subj)
		return "", false
	}
	if ev.File == "" {
		return "", false
	}
	return ev.File, true
}

// ResumeFromLastPlayed tries to resume the clip this instance was playing
// before its last restart, read from the JetStream last-value cache. Returns
// true when playback started; false means the caller should fall through to
// the next startup pick (PlayRandom). A file that has since left the playlist
// (corpus changed under the PVC) falls through too.
func (s *Server) ResumeFromLastPlayed(ctx context.Context) bool {
	file, ok := lastPlayedFile(ctx, natsclient.JetStream(), c.Conf.Environment, c.Conf.Platform)
	if !ok {
		return false
	}
	if err := s.PlayVideoFile(file); err != nil {
		slog.WarnContext(ctx, "lastplayed resume failed; falling back", "err", err, "video", file)
		return false
	}
	slog.InfoContext(ctx, "resumed playback from jetstream lastplayed", "video", file, "platform", c.Conf.Platform)
	return true
}
