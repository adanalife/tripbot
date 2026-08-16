package main

import (
	"context"
	"errors"
	"log/slog"

	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/events"
)

// This file wires the ops-transition event writers (event-taxonomy ADR: the
// events table is the permanent record; metrics retention expires) into the
// binary's startup and watchdog seams. The wiring lives in cmd rather than in
// the packages that observe the transitions, so pkg/obs/watchdog stays free of
// the events/database dependency (package-boundary-init-discipline).

// recordDeploy lands this binary's deploy event in the permanent activity
// log. events.Deploy skips the write when the version matches the platform's
// most recent deploy event for the component, so a pod restart on the same
// build records nothing. Best-effort: a read-only instance skips silently,
// and a failed write logs and drops the event rather than disturbing startup.
func (t *Tripbot) recordDeploy(ctx context.Context) {
	recorded, err := events.Deploy(ctx, t.cfg, "tripbot", t.version)
	switch {
	case errors.Is(err, terrors.ErrReadOnly):
	case err != nil:
		slog.ErrorContext(ctx, "error recording deploy event", "err", err)
	case recorded:
		slog.InfoContext(ctx, "recorded deploy event", "component", "tripbot", "version", t.version)
	}
}

// watchdogRestartHook adapts events.WatchdogRestart to the watchdog's
// OnRestart callback: each forced restart lands as a watchdog_restart event
// naming the watchdog and whether the restart action succeeded.
func (t *Tripbot) watchdogRestartHook(name string) func(context.Context, error) {
	return func(ctx context.Context, restartErr error) {
		outcome := events.WatchdogOutcomeOK
		if restartErr != nil {
			outcome = events.WatchdogOutcomeFailed
		}
		if err := events.WatchdogRestart(ctx, t.cfg, name, outcome); err != nil && !errors.Is(err, terrors.ErrReadOnly) {
			slog.ErrorContext(ctx, "error recording watchdog restart event", "err", err)
		}
	}
}

// watchdogRecoveredHook adapts events.WatchdogRecovered to the watchdog's
// OnRecovered callback: a recovery observed to hold lands as a
// watchdog_recovered event, closing the outage the watchdog_restart opened.
func (t *Tripbot) watchdogRecoveredHook(name string) func(context.Context) {
	return func(ctx context.Context) {
		if err := events.WatchdogRecovered(ctx, t.cfg, name); err != nil && !errors.Is(err, terrors.ErrReadOnly) {
			slog.ErrorContext(ctx, "error recording watchdog recovery event", "err", err)
		}
	}
}
