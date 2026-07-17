package onscreensServer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	c "github.com/adanalife/tripbot/pkg/config/onscreens-server"
	"github.com/adanalife/tripbot/pkg/natsclient"
	oe "github.com/adanalife/tripbot/pkg/onscreens-events"
	"github.com/nats-io/nats.go/jetstream"
)

// middleStateStreamName is the JetStream stream holding the middle-text
// overlay's last-published state — a last-value cache (MaxMsgsPerSubject=1)
// over the single MiddleStateSubject. A restarted onscreens-server reads it
// back to restore whatever text was on screen before the restart, so the
// permanent middle overlay survives a reboot. Same shape as the playout
// TRIPBOT_VLC_LASTPLAYED cache.
const middleStateStreamName = "TRIPBOT_ONSCREENS_MIDDLE"

// EnsureMiddleStateStream idempotently declares the middle-text state stream.
// Call it once at startup before the first show/hide is handled — a core
// publish to a subject no stream covers is silently uncaptured, so the stream
// has to exist before the first command for restore-on-restart to have
// anything to read. No-op when js is nil (NATS off / JetStream unavailable);
// the overlay then degrades to its previous behaviour (text lost on restart).
func EnsureMiddleStateStream(ctx context.Context, js jetstream.JetStream, env string) error {
	if js == nil {
		return nil
	}
	cfg := jetstream.StreamConfig{
		Name:        middleStateStreamName,
		Description: "Last middle-text overlay state (content + visibility) for restore-on-restart.",
		// Wildcard over every platform leaf so both onscreens-twitch and
		// onscreens-youtube can idempotently ensure the one stream while each
		// reads/writes only its own tripbot.<env>.onscreens.middle.state.<platform>.
		Subjects:  []string{oe.MiddleStateWildcard(env)},
		Storage:   jetstream.FileStorage,
		Retention: jetstream.LimitsPolicy,
		Discard:   jetstream.DiscardOld,
		// One retained message: exactly the latest middle-text state, nothing
		// to prune.
		MaxMsgsPerSubject: 1,
	}
	if _, err := js.CreateOrUpdateStream(ctx, cfg); err != nil {
		return fmt.Errorf("ensure stream %s: %w", middleStateStreamName, err)
	}
	slog.InfoContext(ctx, "jetstream stream ensured", "stream", middleStateStreamName, "env", env)
	return nil
}

// publishMiddleState publishes the middle overlay's current content +
// visibility as the last-value state. Fire-and-forget core publish — the
// stream captures it; when NATS is unconfigured it's a silent no-op (matching
// the eventbus convention).
func publishMiddleState(ctx context.Context, env, platform, msg string, showing bool) {
	conn := natsclient.Conn()
	if conn == nil {
		return
	}
	payload, err := json.Marshal(oe.MiddleState{Envelope: oe.NewEnvelope(), Msg: msg, Showing: showing})
	if err != nil {
		slog.ErrorContext(ctx, "middle state marshal failed", "err", err)
		return
	}
	subj := oe.MiddleStateSubject(env, platform)
	if err := conn.Publish(subj, payload); err != nil {
		slog.ErrorContext(ctx, "middle state publish failed", "err", err, "subject", subj)
	}
}

// readMiddleState reads the last-value cache back: the content + visibility
// onscreens-server most recently published, or ok=false when there's nothing
// to restore (empty stream, NATS off, or any read error — restore is
// best-effort, so errors are logged and swallowed).
func readMiddleState(ctx context.Context, js jetstream.JetStream, env, platform string) (msg string, showing, ok bool) {
	if js == nil {
		return "", false, false
	}
	stream, err := js.Stream(ctx, middleStateStreamName)
	if err != nil {
		slog.WarnContext(ctx, "middle state stream lookup failed", "err", err, "stream", middleStateStreamName)
		return "", false, false
	}
	subj := oe.MiddleStateSubject(env, platform)
	raw, err := stream.GetLastMsgForSubject(ctx, subj)
	if err != nil {
		if !errors.Is(err, jetstream.ErrMsgNotFound) {
			slog.WarnContext(ctx, "middle state read failed", "err", err, "subject", subj)
		}
		return "", false, false
	}
	var ev oe.MiddleState
	if err := json.Unmarshal(raw.Data, &ev); err != nil {
		slog.WarnContext(ctx, "middle state decode failed", "err", err, "subject", subj)
		return "", false, false
	}
	return ev.Msg, ev.Showing, true
}

// RestoreMiddleText ensures the state stream exists, then restores the
// middle-text overlay's content + visibility from the JetStream last-value
// cache so the permanent overlay survives an onscreens-server restart.
// Best-effort: a nil JetStream (NATS off) or an empty cache leaves the
// overlay at its constructed default (empty content, showing).
func (s *Server) RestoreMiddleText(ctx context.Context) {
	js := natsclient.JetStream()
	if err := EnsureMiddleStateStream(ctx, js, c.Conf.Environment); err != nil {
		slog.ErrorContext(ctx, "couldn't ensure middle state stream", "err", err)
		return
	}
	msg, showing, ok := readMiddleState(ctx, js, c.Conf.Environment, c.Conf.Platform)
	if !ok {
		return
	}
	s.MiddleText.Content = msg
	s.MiddleText.IsShowing = showing
	slog.InfoContext(ctx, "restored middle-text overlay from jetstream", "showing", showing)
}
