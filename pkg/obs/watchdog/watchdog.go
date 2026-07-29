// Package watchdog implements the silent-disconnect detector that
// cross-checks OBS's outputActive state against the platform's live
// status and forces recovery on sustained divergence. Lives here (not
// in the parent pkg/obs package) so binaries that only need pkg/obs's
// WebSocket helpers don't drag in pkg/config/tripbot or pkg/twitch
// transitively. cmd/tripbot is the sole consumer.
//
// The recovery action is the caller's to supply: restarting the OBS output
// fixes a half-open RTMP socket, but a platform that mints a broadcast object
// per session needs that object re-minted instead.
package watchdog

import (
	"context"
	"log/slog"
	"time"

	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/adanalife/tripbot/pkg/obs"
)

// WatchdogDeps are the OBS+platform+recovery hooks the silent-disconnect
// watchdog calls. Injectable so the loop can be unit-tested without a real
// OBS WebSocket or live platform client. DefaultWatchdogDeps wires the OBS +
// restart hooks; ChannelLive is injected by the caller.
type WatchdogDeps struct {
	OBSActive func(context.Context) (bool, error)
	// ChannelLive reports whether the channel is live. Injected by cmd/tripbot,
	// which routes it through the platform-gateway — this package must not reach
	// a platform API itself (package-boundary-init-discipline), so
	// DefaultWatchdogDeps leaves it nil.
	ChannelLive func(context.Context) (bool, error)
	// Restart returns the stream to a viewable state. Defaults to restarting the
	// OBS output; a caller whose platform needs a heavier recovery replaces it.
	Restart func(context.Context) error
}

// DefaultWatchdogDeps wires WatchSilentDisconnect's OBS + restart hooks. The
// caller injects ChannelLive (the gateway live-check).
//
// OBSActive uses GetStreamActiveSteady (not GetStreamStatus) so the
// watchdog skips counting misses when OBS already knows the stream is
// failing — outputReconnecting=true means OBS will handle recovery
// itself, and a watchdog-forced restart there would just race OBS's
// reconnect. Only the truly silent half-open (outputActive=true AND
// outputReconnecting=false AND Twitch offline) needs intervention.
func DefaultWatchdogDeps() WatchdogDeps {
	return WatchdogDeps{
		OBSActive: obs.GetStreamActiveSteady,
		Restart:   defaultRestart,
	}
}

// defaultRestart stops then starts the OBS stream with a small gap to let
// the RTMP teardown settle before OBS opens a fresh connection. Matches
// the manual recovery sequence we ran by hand the first time the silent
// half-open hit prod (see the 2026-05-27 incident).
func defaultRestart(ctx context.Context) error {
	if err := obs.StopStream(ctx); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(3 * time.Second):
	}
	return obs.StartStream(ctx)
}

// WatchSilentDisconnect detects the state where OBS reports outputActive=true
// but the platform reports the channel offline, and calls deps.Restart after
// `threshold` consecutive misalignments. `cooldown` bounds how often recovery
// can fire so a flapping platform API can't put us in a restart loop — but it
// is retired as soon as a recovery is seen to hold (see below), so it only ever
// suppresses retries of a recovery that isn't working.
//
// Background: when Twitch's ingest server closes the RTMP session without
// the FIN/RST making it back to OBS (e.g. an idle middlebox dropping the
// connection, or some Twitch-side terminations), OBS's write socket stays
// open and its built-in reconnect never fires — it keeps writing into the
// void. TikTok reaches the same place by a different route: the Streamlabs
// room is reaped once a push gap outlives the relay target's idleTimeout,
// leaving OBS pushing happily at a room nobody can watch. Either way the
// divergence is only visible from outside OBS.
func WatchSilentDisconnect(ctx context.Context, deps WatchdogDeps, interval time.Duration, threshold int, cooldown time.Duration) {
	misses, liveStreak := 0, 0
	var lastRestart time.Time
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	slog.InfoContext(ctx, "obs silent-disconnect watchdog started",
		"interval", interval, "threshold", threshold, "cooldown", cooldown)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			obsActive, err := deps.OBSActive(ctx)
			if err != nil {
				// OBS unreachable — let PollStreamingActive's gauge be the
				// alert signal. Reset misses so a transient OBS blip doesn't
				// race with a real Twitch-side drop.
				slog.WarnContext(ctx, "watchdog: obs status unavailable", "err", err)
				misses = 0
				continue
			}
			if !obsActive {
				// OBS itself isn't streaming — nothing for the watchdog to
				// do. The operator-driven "start streaming" gesture is
				// outside our scope.
				misses = 0
				continue
			}
			live, err := deps.ChannelLive(ctx)
			if err != nil {
				slog.WarnContext(ctx, "watchdog: live status unavailable", "err", err)
				misses = 0
				continue
			}
			if live {
				misses = 0
				// A recovery that has held for as long as it takes to declare a
				// death took. The cooldown exists to stop retrying a recovery
				// that isn't working, so holding it against a later, unrelated
				// outage only keeps the stream dark: on 2026-07-29 TikTok ended
				// the LIVE 21 minutes after a re-mint, and the 30m cooldown
				// stranded it for 5 more minutes past detection.
				liveStreak++
				if liveStreak >= threshold && !lastRestart.IsZero() {
					slog.InfoContext(ctx, "watchdog: recovery held, cooldown retired",
						"live_ticks", liveStreak)
					lastRestart = time.Time{}
				}
				continue
			}
			liveStreak = 0
			misses++
			slog.WarnContext(ctx, "watchdog: silent-disconnect suspected",
				"misses", misses, "threshold", threshold)
			if misses < threshold {
				continue
			}
			if since := time.Since(lastRestart); since < cooldown {
				slog.WarnContext(ctx, "watchdog: restart suppressed by cooldown",
					"since_last_restart", since, "cooldown", cooldown)
				continue
			}
			slog.ErrorContext(ctx, "watchdog: forcing stream restart",
				"consecutive_misses", misses)
			if err := deps.Restart(ctx); err != nil {
				slog.ErrorContext(ctx, "watchdog: restart failed", "err", err)
				continue
			}
			instrumentation.OBSSilentDisconnectRestarts.Inc()
			lastRestart = time.Now()
			misses = 0
		}
	}
}
