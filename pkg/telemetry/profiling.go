package telemetry

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/grafana/pyroscope-go"
)

// startProfiler wires up the Pyroscope continuous-profiling agent so CPU
// / heap / goroutine / mutex / block profiles get pushed to Grafana Cloud
// Profiles on a regular cadence. Goroutine leaks and slow handlers become
// forensic-friendly without needing to reproduce live.
//
// Configuration comes from env vars (matches the field names in Grafana
// Cloud's "Configure the client" panel so seeding from the UI is
// copy-paste):
//
//   - PYROSCOPE_SERVER_ADDRESS    — endpoint URL; if unset, profiling is
//     skipped entirely so local dev / disabled-telemetry runs are no-ops.
//   - PYROSCOPE_BASIC_AUTH_USER     — Grafana Cloud stack ID.
//   - PYROSCOPE_BASIC_AUTH_PASSWORD — Grafana Cloud API token.
//
// Returns a stop function safe to call unconditionally.
func startProfiler(serviceName, serviceVersion string) (func(), error) {
	addr := os.Getenv("PYROSCOPE_SERVER_ADDRESS")
	if addr == "" {
		slog.Info("pyroscope: PYROSCOPE_SERVER_ADDRESS unset, continuous profiling disabled")
		return func() {}, nil
	}

	p, err := pyroscope.Start(pyroscope.Config{
		ApplicationName:   serviceName,
		ServerAddress:     addr,
		BasicAuthUser:     os.Getenv("PYROSCOPE_BASIC_AUTH_USER"),
		BasicAuthPassword: os.Getenv("PYROSCOPE_BASIC_AUTH_PASSWORD"),
		Tags: map[string]string{
			"version": serviceVersion,
		},
		ProfileTypes: []pyroscope.ProfileType{
			pyroscope.ProfileCPU,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileAllocSpace,
			pyroscope.ProfileInuseObjects,
			pyroscope.ProfileInuseSpace,
			pyroscope.ProfileGoroutines,
			pyroscope.ProfileMutexCount,
			pyroscope.ProfileMutexDuration,
			pyroscope.ProfileBlockCount,
			pyroscope.ProfileBlockDuration,
		},
	})
	if err != nil {
		return func() {}, fmt.Errorf("pyroscope.Start: %w", err)
	}
	slog.Info("pyroscope: continuous profiling enabled", "service", serviceName, "endpoint", addr)
	return func() {
		if err := p.Stop(); err != nil {
			slog.Warn("pyroscope: stop failed", "err", err)
		}
	}, nil
}
