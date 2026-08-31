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
	"fmt"
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
	// Platform labels this leg's metrics (twitch, tiktok, youtube). Every
	// per-platform instance shares one counter, and service.platform lives only
	// on the OTel resource, so without it the legs collide onto a single series
	// — the same reason every OBS metric stamps it. Empty defaults to twitch.
	Platform string

	// OBSState reports OBS's streaming output as steady / reconnecting /
	// inactive. Three states rather than a boolean because the loop treats
	// reconnecting differently from both: see the reconnect grace in
	// WatchSilentDisconnect.
	OBSState func(context.Context) (obs.StreamState, error)
	// ChannelLive reports whether the channel is live. Injected by cmd/tripbot,
	// which routes it through the platform-gateway — this package must not reach
	// a platform API itself (package-boundary-init-discipline), so
	// DefaultWatchdogDeps leaves it nil.
	ChannelLive func(context.Context) (bool, error)
	// Restart returns the stream to a viewable state. Defaults to restarting the
	// OBS output; a caller whose platform needs a heavier recovery replaces it.
	Restart func(context.Context) error

	// OnRestart, when non-nil, is told about each forced restart right after
	// Restart returns — restartErr is Restart's error, nil on success. cmd/tripbot
	// wires it to the permanent events log; the hook lives here as a callback so
	// this package takes no events/database dependency
	// (package-boundary-init-discipline).
	OnRestart func(ctx context.Context, restartErr error)
	// OnRecovered, when non-nil, is told when a watchdog-forced recovery is seen
	// to hold — the same transition that retires the restart cooldown.
	OnRecovered func(ctx context.Context)
}

// DefaultWatchdogDeps wires WatchSilentDisconnect's OBS + restart hooks. The
// caller injects ChannelLive (the gateway live-check).
func DefaultWatchdogDeps() WatchdogDeps {
	return WatchdogDeps{
		OBSState: obs.GetStreamState,
		Restart:  RestartOBSOutput,
	}
}

// RestartOBSOutput stops the OBS stream, waits for the output to actually go
// down, then lets the RTMP teardown settle before starting a fresh one. Matches
// the manual recovery sequence we ran by hand the first time the silent
// half-open hit prod (see the 2026-05-27 incident). Exported because a
// platform whose recovery replaces Restart still needs the push itself
// re-established afterwards.
func RestartOBSOutput(ctx context.Context) error {
	if err := obs.StopStream(ctx); err != nil {
		return err
	}
	if err := awaitOutputStopped(ctx, obs.GetStreamStatus); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(teardownSettle):
	}
	return obs.StartStream(ctx)
}

// How long to wait for OBS to report the output stopped, how often to ask, and
// the pause between a confirmed stop and the fresh StartStream.
const (
	stopTimeout    = 30 * time.Second
	stopPoll       = 500 * time.Millisecond
	teardownSettle = 3 * time.Second
)

// awaitOutputStopped blocks until OBS reports the output inactive, so the
// StartStream that follows can't land on a teardown still in flight.
//
// obs-websocket answers StopStream as soon as the stop is *initiated* — the
// output then sits in OUTPUT_STOPPING until the RTMP socket actually closes. On
// the half-open socket this watchdog exists to catch, that close is exactly the
// thing that isn't completing promptly, so the teardown routinely outlives a
// short pause, and a StartStream issued against a still-stopping output is
// rejected with request status 500, OutputRunning. The stream stays dark and
// the retry re-stops an output that was already going down.
//
// active is obs.GetStreamStatus in production, injected so the poll is
// testable without an OBS WebSocket. It must be the raw outputActive read and
// not a steady-state one: a reconnecting output is still active, and treating
// it as stopped would let StartStream race the very teardown this waits out.
func awaitOutputStopped(ctx context.Context, active func(context.Context) (bool, error)) error {
	deadline := time.Now().Add(stopTimeout)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(stopPoll):
		}
		stillActive, err := active(ctx)
		if err != nil {
			return err
		}
		if !stillActive {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("obs output still active %s after StopStream", stopTimeout)
		}
	}
}

// reconnectGrace is how long OBS's own reconnect is left alone before the
// watchdog starts treating a reconnecting output as an active one. It exists
// because OBS's reconnect is not guaranteed to retry: on 2026-08-28 the Twitch
// RTMP session died, OBS logged one "Reconnecting in 2.00 seconds..", never
// attempted again, and the stream stayed dark for 31 minutes while the watchdog
// zeroed its counter every tick. Three minutes is well clear of a genuine
// reconnect — those land in seconds — and still bounded well inside the time it
// took anyone to notice by hand.
const reconnectGrace = 3 * time.Minute

// WatchSilentDisconnect detects the state where OBS reports outputActive=true
// but the platform reports the channel offline, and calls deps.Restart after
// `threshold` consecutive misalignments. `cooldown` bounds how often recovery
// can fire so a flapping platform API can't put us in a restart loop — but it
// is retired as soon as a recovery is seen to hold (see below), so it only ever
// suppresses retries of a recovery that isn't working. A live-check that errors
// answers neither way: collected misses are held (and aged out) rather than
// cleared, so an unreliable check can't mask a channel that is genuinely dark.
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
	var lastRestart, lastAnswer, reconnectingSince time.Time
	// How long collected misses outlive the last definite live/offline answer:
	// the same span it takes to declare a death, so evidence never survives
	// longer than it would have taken to act on it.
	staleAfter := time.Duration(threshold) * interval
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	slog.InfoContext(ctx, "obs silent-disconnect watchdog started",
		"interval", interval, "threshold", threshold, "cooldown", cooldown)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			obsState, err := deps.OBSState(ctx)
			if err != nil {
				// OBS unreachable — let PollStreamingActive's gauge be the
				// alert signal. Reset misses so a transient OBS blip doesn't
				// race with a real Twitch-side drop.
				slog.WarnContext(ctx, "watchdog: obs status unavailable", "err", err)
				misses = 0
				continue
			}
			if obsState == obs.StreamInactive {
				// OBS itself isn't streaming — nothing for the watchdog to
				// do. The operator-driven "start streaming" gesture is
				// outside our scope.
				misses, reconnectingSince = 0, time.Time{}
				continue
			}
			if obsState == obs.StreamReconnecting {
				// OBS has admitted the session failed and is retrying. Stand
				// down only for as long as its own retry deserves the chance:
				// past reconnectGrace, treat the output like any other active
				// one and let the live-check decide, because a reconnect that
				// has not landed by now is not recovery in progress.
				if reconnectingSince.IsZero() {
					reconnectingSince = time.Now()
				}
				if since := time.Since(reconnectingSince); since < reconnectGrace {
					// Misses are held rather than zeroed: a reconnect arriving
					// mid-divergence is more evidence of a dark channel, not a
					// reason to forget what was already counted.
					slog.WarnContext(ctx, "watchdog: obs reconnecting, holding",
						"reconnecting_for", since, "grace", reconnectGrace,
						"misses", misses)
					continue
				}
			} else {
				reconnectingSince = time.Time{}
			}
			live, err := deps.ChannelLive(ctx)
			if err != nil {
				// An unknown answer is not evidence of health. Zeroing here let a
				// live-check failing every other tick hold misses under the
				// threshold indefinitely, so a dark channel would never be
				// recovered — and TikTok's check is least reliable exactly when
				// rooms are churning, which is when recovery matters most. Keep
				// the count and let it go stale instead, so an unreliable check
				// delays recovery rather than preventing it.
				if time.Since(lastAnswer) > staleAfter {
					misses = 0
				}
				slog.WarnContext(ctx, "watchdog: live status unavailable",
					"err", err, "misses", misses)
				continue
			}
			lastAnswer = time.Now()
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
					if deps.OnRecovered != nil {
						deps.OnRecovered(ctx)
					}
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
			// Stamped on the attempt rather than on success, so the cooldown
			// governs a recovery that keeps failing — the case it exists for.
			// Stamping it after the error check left a failing Restart with no
			// timestamp to measure against, so it re-fired every tick: OBS took
			// longer to tear the output down than the restart waited, and the
			// resulting StartStream rejection retried 24 times in 9 hours,
			// each attempt re-stopping an output already mid-teardown.
			lastRestart = time.Now()
			// A forced Stop+Start replaces whatever reconnect was in flight, so
			// the next one starts its grace over rather than inheriting this
			// one's age.
			reconnectingSince = time.Time{}
			restartErr := deps.Restart(ctx)
			// Recorded before the error check, so a recovery that keeps failing
			// is visible. Counting only successes made the worse outage the
			// quieter one: on 2026-08-05 every attempt failed for 9h41m and the
			// counter never moved, so the panel over it read zero throughout.
			instrumentation.OBSSilentDisconnectRestarts.Attempt(deps.Platform, restartErr)
			if deps.OnRestart != nil {
				deps.OnRestart(ctx, restartErr)
			}
			if restartErr != nil {
				slog.ErrorContext(ctx, "watchdog: restart failed", "err", restartErr)
				continue
			}
			misses = 0
		}
	}
}
