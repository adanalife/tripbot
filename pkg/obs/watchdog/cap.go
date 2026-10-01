package watchdog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/adanalife/tripbot/pkg/obs"
)

// BroadcastCapDeps are the hooks WatchBroadcastCap calls. Injectable so the
// loop is testable without OBS or a platform.
type BroadcastCapDeps struct {
	// Platform labels the log lines.
	Platform string
	// OBSState reports OBS's output state. The loop only acts on a steady
	// output: a stopped one is the operator's, and a reconnecting one is the
	// silent-disconnect watchdog's.
	OBSState func(context.Context) (obs.StreamState, error)
	// StartedAt reports when the platform's running broadcast began. Zero means
	// not live or unknown, and the loop leaves a zero alone. It must be the
	// platform's clock and not OBS's: OBS's output duration resets on a quick
	// reconnect that the platform resumes as the same broadcast, so it
	// under-counts exactly the sessions the silent-disconnect watchdog bounced.
	StartedAt func(context.Context) (time.Time, error)
	// Restart ends the running broadcast and starts a fresh one.
	Restart func(context.Context) error
	// OnRestart, when non-nil, is told about each restart right after Restart
	// returns, with Restart's error.
	OnRestart func(ctx context.Context, restartErr error)
}

// WatchBroadcastCap restarts the stream once the platform's running broadcast
// is maxAge old, so a platform that ends broadcasts at a fixed length (Twitch,
// at 48 hours) gets a short planned gap instead of an abrupt cut.
//
// It restarts at most once per broadcast. If a restart leaves the platform
// reporting the same start time, the platform resumed the old broadcast rather
// than opening a new one, and restarting again would only add gaps before the
// same cut. The loop logs that once and waits for the next broadcast.
func WatchBroadcastCap(ctx context.Context, deps BroadcastCapDeps, interval, maxAge time.Duration) {
	// restartedFor is the start time of the broadcast this loop last restarted,
	// and warnedFor the one it last logged as kept.
	var restartedFor, warnedFor time.Time
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	slog.InfoContext(ctx, "broadcast-cap watchdog started",
		"platform", deps.Platform, "interval", interval, "max_age", maxAge)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		state, err := deps.OBSState(ctx)
		if err != nil || state != obs.StreamSteady {
			continue
		}
		started, err := deps.StartedAt(ctx)
		if err != nil {
			slog.WarnContext(ctx, "broadcast-cap watchdog: start time unavailable",
				"platform", deps.Platform, "err", err)
			continue
		}
		if started.IsZero() {
			continue
		}
		age := time.Since(started)
		if age < maxAge {
			continue
		}
		if started.Equal(restartedFor) {
			if !started.Equal(warnedFor) {
				warnedFor = started
				slog.WarnContext(ctx, "broadcast-cap watchdog: restart kept the same broadcast, leaving it",
					"platform", deps.Platform, "started_at", started, "age", age)
			}
			continue
		}
		restartedFor = started
		slog.InfoContext(ctx, "broadcast-cap watchdog: restarting before the platform's cap",
			"platform", deps.Platform, "started_at", started, "age", age, "max_age", maxAge)
		restartErr := deps.Restart(ctx)
		if deps.OnRestart != nil {
			deps.OnRestart(ctx, restartErr)
		}
		if restartErr != nil {
			slog.ErrorContext(ctx, "broadcast-cap watchdog: restart failed",
				"platform", deps.Platform, "err", restartErr)
		}
	}
}

// How long a broadcast restart waits for the platform to report the channel
// offline, how often it asks, and how many times it tries to start the output
// again afterwards.
//
// The wait bounds the gap viewers see. Five minutes stays inside the
// "not broadcasting for 10m" alert, and a platform that still reports live
// after it gets the output back anyway: a gap with no fresh broadcast is
// worse than no gap.
const (
	offlineTimeout = 5 * time.Minute
	offlinePoll    = 15 * time.Second
	startAttempts  = 3
	startRetry     = 5 * time.Second
	startTimeout   = time.Minute
)

// RestartBroadcast stops the OBS output, waits for the platform to report the
// channel offline, then starts the output again. Waiting for offline is what
// makes the platform open a new broadcast: RestartOBSOutput's 3-second pause
// fixes a half-open socket, but a platform can resume a broadcast across a
// gap that short.
//
// Once the output is stopped, the start runs even if ctx is cancelled. A pod
// shutting down mid-restart must not leave OBS stopped, because nothing else
// starts it again.
func RestartBroadcast(ctx context.Context, live func(context.Context) (bool, error)) error {
	if err := stopOBSOutput(ctx); err != nil {
		return fmt.Errorf("stop: %w", err)
	}
	if err := awaitOffline(ctx, live, offlineTimeout, offlinePoll); err != nil {
		slog.WarnContext(ctx, "broadcast restart: starting the output without an offline reading", "err", err)
	}
	startCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), startTimeout)
	defer cancel()
	return retryStart(startCtx, startOBSOutput, startAttempts, startRetry)
}

func stopOBSOutput(ctx context.Context) error {
	client, err := obs.Dial(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err := client.Disconnect(); err != nil {
			slog.WarnContext(ctx, "obs disconnect", "err", err)
		}
	}()
	if err := obs.StopStreamOn(client); err != nil {
		return err
	}
	active := func(context.Context) (bool, error) { return obs.StreamActiveOn(client) }
	return awaitOutputStopped(ctx, active)
}

func startOBSOutput(ctx context.Context) error {
	client, err := obs.Dial(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err := client.Disconnect(); err != nil {
			slog.WarnContext(ctx, "obs disconnect", "err", err)
		}
	}()
	return obs.StartStreamOn(client)
}

// awaitOffline polls live until it answers false or timeout passes. A failed
// read counts as not-yet-offline rather than ending the wait.
func awaitOffline(ctx context.Context, live func(context.Context) (bool, error), timeout, poll time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(poll):
		}
		isLive, err := live(ctx)
		if err == nil && !isLive {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("platform still reports live %s after the output stopped", timeout)
		}
	}
}

// retryStart calls start up to attempts times, pausing between tries.
func retryStart(ctx context.Context, start func(context.Context) error, attempts int, pause time.Duration) error {
	var errs []error
	for i := range attempts {
		if i > 0 {
			select {
			case <-ctx.Done():
				return errors.Join(append(errs, ctx.Err())...)
			case <-time.After(pause):
			}
		}
		err := start(ctx)
		if err == nil {
			return nil
		}
		errs = append(errs, err)
	}
	return fmt.Errorf("start output: %w", errors.Join(errs...))
}
