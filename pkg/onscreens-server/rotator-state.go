package onscreensServer

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/adanalife/tripbot/pkg/natsclient"
	oe "github.com/adanalife/tripbot/pkg/onscreens-events"
	rot "github.com/adanalife/tripbot/pkg/rotator"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// rotatorConfigStreamDescription documents the stream in `nats stream ls`.
const rotatorConfigStreamDescription = "Last admin-console rotator copy per platform, for restore-on-restart."

// EnsureRotatorConfigStream idempotently declares the rotator-copy last-value
// stream. tripbot ensures it before publishing too — whichever side starts first
// declares it, because a core publish to a subject no stream covers is silently
// uncaptured. No-op without JetStream.
func EnsureRotatorConfigStream(ctx context.Context, js jetstream.JetStream, env string) error {
	return natsclient.EnsureLastValueStream(ctx, js,
		oe.RotatorConfigStreamName, rotatorConfigStreamDescription,
		[]string{oe.RotatorConfigWildcard(env)})
}

// handleRotatorConfig applies copy pushed from tripbot when the console saves.
// Strict about decoding (a malformed body is a publisher bug, not an intent) but
// lenient about content: an empty pool is a legitimate edit for a platform that
// should show nothing in a corner, and rot.Pick returns "" for an empty pool
// rather than erroring.
func (s *Server) handleRotatorConfig(m *nats.Msg) {
	var ev oe.RotatorConfig
	if err := json.Unmarshal(m.Data, &ev); err != nil {
		slog.Error("nats: decode rotator.config", "err", err, "subject", m.Subject)
		return
	}
	s.applyRotatorConfig(rot.Config{Left: ev.Left, Right: ev.Right, RareMessage: ev.RareMessage})
	slog.Info("applied rotator copy from console",
		"left_messages", len(ev.Left.Messages), "right_messages", len(ev.Right.Messages),
		"left_promo", len(ev.Left.PromoMessages), "right_promo", len(ev.Right.PromoMessages))
}

// readRotatorConfig reads this platform's copy back from the last-value cache,
// or ok=false when there's nothing stored (empty stream, NATS off, or any read
// error — restore is best-effort, so errors are logged and swallowed).
func readRotatorConfig(ctx context.Context, js jetstream.JetStream, env, platform string) (cfg rot.Config, ok bool) {
	if js == nil {
		return rot.Config{}, false
	}
	stream, err := js.Stream(ctx, oe.RotatorConfigStreamName)
	if err != nil {
		slog.WarnContext(ctx, "rotator config stream lookup failed", "err", err,
			"stream", oe.RotatorConfigStreamName)
		return rot.Config{}, false
	}
	subj := oe.RotatorConfigSubject(env, platform)
	raw, err := stream.GetLastMsgForSubject(ctx, subj)
	if err != nil {
		if !errors.Is(err, jetstream.ErrMsgNotFound) {
			slog.WarnContext(ctx, "rotator config read failed", "err", err, "subject", subj)
		}
		return rot.Config{}, false
	}
	var ev oe.RotatorConfig
	if err := json.Unmarshal(raw.Data, &ev); err != nil {
		slog.WarnContext(ctx, "rotator config decode failed", "err", err, "subject", subj)
		return rot.Config{}, false
	}
	return rot.Config{Left: ev.Left, Right: ev.Right, RareMessage: ev.RareMessage}, true
}

// RestoreRotatorCopy ensures the stream exists, then restores this platform's
// console-edited rotator copy from the JetStream last-value cache so a restart
// doesn't drop the overlays back to the copy compiled into the binary.
//
// Best-effort by design: a nil JetStream (NATS off) or an empty cache leaves the
// rotators on rot.DefaultConfig, which is always valid copy. Postgres remains
// the record of truth — tripbot republishes on its own startup, so even a wiped
// JetStream volume self-heals rather than losing the edits.
func (s *Server) RestoreRotatorCopy(ctx context.Context) {
	js := natsclient.JetStream()
	if err := EnsureRotatorConfigStream(ctx, js, s.cfg.Environment); err != nil {
		// Warn rather than error: the only way this fails in practice is NATS
		// not being reachable yet at boot, and the restore is best-effort — the
		// copy comes back on tripbot's next publish.
		slog.WarnContext(ctx, "couldn't ensure rotator config stream", "err", err)
		return
	}
	cfg, ok := readRotatorConfig(ctx, js, s.cfg.Environment, s.cfg.Platform)
	if !ok {
		return
	}
	s.applyRotatorConfig(cfg)
	slog.InfoContext(ctx, "restored rotator copy from jetstream",
		"left_messages", len(cfg.Left.Messages), "right_messages", len(cfg.Right.Messages))
}
