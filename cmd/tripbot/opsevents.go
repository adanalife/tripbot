package main

import (
	"context"
	"errors"
	"log/slog"
	"time"

	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/events"
	"github.com/adanalife/tripbot/pkg/instrumentation"
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

// orphanCheckDelay is how long after startup the orphaned-session count is
// taken. A rolling update starts this pod while the outgoing one is still
// draining, and the outgoing pod's graceful shutdown writes its logouts during
// that overlap; counting immediately would read those live sessions as
// orphans. Two minutes clears the drain with room to spare.
const orphanCheckDelay = 2 * time.Minute

// reportOrphanedSessions answers, once per boot, the question a reboot alert
// never does — what did the last exit cost? It counts the platform's login
// events from before this process started that no logout ever paired, and
// records the count as a gauge and a log line. A graceful exit pairs every
// session and reports 0; anything else names how many viewers had in-flight
// miles discarded, so the loss is known within minutes instead of when a
// viewer remembers their own number. startedAt is the process start, so
// sessions this run opens are never counted.
func (t *Tripbot) reportOrphanedSessions(ctx context.Context, startedAt time.Time) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(orphanCheckDelay):
	}
	n, err := events.OrphanedSessions(ctx, t.cfg.Platform, startedAt)
	if err != nil {
		slog.ErrorContext(ctx, "error counting orphaned sessions", "err", err)
		return
	}
	instrumentation.OrphanedSessions.Set(n, t.cfg.Platform)
	if n > 0 {
		slog.WarnContext(ctx, "previous exit was ungraceful: sessions left without a logout", "orphaned_sessions", n)
		return
	}
	slog.InfoContext(ctx, "previous exit paired every session", "orphaned_sessions", 0)
}
