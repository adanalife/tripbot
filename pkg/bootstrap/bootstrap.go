// Package bootstrap holds the boot/shutdown spine shared by the cmd/
// binaries, so signal handling, telemetry bring-up, and exporter flushing
// live in one place instead of drifting copies per binary.
package bootstrap

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adanalife/tripbot/pkg/config"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/telemetry"
	"github.com/getsentry/sentry-go"
)

// flushTimeout bounds each exporter flush at exit. The whole shutdown path
// should complete well inside a pod's termination grace period so the
// supervisor never has to SIGKILL mid-drain.
const flushTimeout = 5 * time.Second

// Start wires the shared boot spine: a context canceled on SIGINT/SIGTERM,
// OpenTelemetry providers (no-op when disabled), and the error logger.
//
// The returned flush func flushes Sentry and telemetry exporters; callers
// defer it in main so it runs after their own cleanup, and the process then
// exits 0 — a caught signal is a clean shutdown, not a failure.
func Start(name, version string, conf config.Config) (context.Context, func()) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	telemetryShutdown, err := telemetry.Init(context.Background(), name, version)
	if err != nil {
		// telemetry init failure shouldn't crash the binary — log and continue.
		slog.Warn("telemetry init failed", "err", err)
	}
	terrors.Initialize(conf, version)

	return ctx, func() {
		stop()
		sentry.Flush(flushTimeout)
		if telemetryShutdown != nil {
			flushCtx, cancel := context.WithTimeout(context.Background(), flushTimeout)
			if err := telemetryShutdown(flushCtx); err != nil {
				slog.Error("telemetry shutdown failed", "err", err)
			}
			cancel()
		}
	}
}
