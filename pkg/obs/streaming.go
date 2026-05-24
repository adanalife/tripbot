// Package obs provides polling integration with the OBS WebSocket API.
package obs

import (
	"context"
	"log/slog"
	"os"
	"time"

	goobs "github.com/andreykaipov/goobs"
	"github.com/adanalife/tripbot/pkg/instrumentation"
)

// PollStreamingActive connects to the OBS WebSocket and updates the
// obs_streaming_active gauge every interval. Intended to be run as a
// long-lived goroutine. Reconnects automatically on connection loss.
func PollStreamingActive(ctx context.Context, interval time.Duration) {
	addr := os.Getenv("OBS_WEBSOCKET_ADDR")
	if addr == "" {
		addr = "obs:4455"
	}
	passwd := os.Getenv("OBS_WEBSOCKET_PASSWD")
	if passwd == "" {
		passwd = "adanalife"
	}

	for {
		if err := ctx.Err(); err != nil {
			return
		}
		poll(ctx, addr, passwd, interval)
		// poll returned — connection lost. Wait before reconnecting.
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

// poll connects once and loops until the context is cancelled or the
// connection drops.
func poll(ctx context.Context, addr, passwd string, interval time.Duration) {
	client, err := goobs.New(addr, goobs.WithPassword(passwd))
	if err != nil {
		slog.ErrorContext(ctx, "obs websocket connect failed", "addr", addr, "err", err)
		instrumentation.OBSStreaming.Set(false)
		return
	}
	defer func() {
		if err := client.Disconnect(); err != nil {
			slog.WarnContext(ctx, "obs disconnect", "err", err)
		}
	}()

	slog.InfoContext(ctx, "obs websocket connected", "addr", addr)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			resp, err := client.Stream.GetStreamStatus()
			if err != nil {
				// Transient: OBS pod restart, network blip, websocket drop.
				// The outer loop reconnects after 10s and OBSStreaming
				// is the alertable signal — keep this off Sentry.
				slog.WarnContext(ctx, "obs GetStreamStatus error", "err", err)
				instrumentation.OBSStreaming.Set(false)
				return // trigger reconnect
			}
			instrumentation.OBSStreaming.Set(resp.OutputActive)
			instrumentation.OBSStats.UpdateStream(instrumentation.OBSStreamSnapshot{
				OutputBytes:      resp.OutputBytes,
				OutputDurationMS: resp.OutputDuration,
				OutputCongestion: resp.OutputCongestion,
				Reconnecting:     resp.OutputReconnecting,
				SkippedFrames:    resp.OutputSkippedFrames,
				TotalFrames:      resp.OutputTotalFrames,
			})

			stats, err := client.General.GetStats()
			if err != nil {
				// Non-fatal — keep the connection alive; stream-side
				// gauges already published this tick.
				slog.WarnContext(ctx, "obs GetStats error", "err", err)
				continue
			}
			instrumentation.OBSStats.Update(instrumentation.OBSStatsSnapshot{
				ActiveFPS:              stats.ActiveFps,
				AverageFrameRenderTime: stats.AverageFrameRenderTime,
				CPUUsage:               stats.CpuUsage,
				MemoryUsage:            stats.MemoryUsage,
				RenderSkippedFrames:    stats.RenderSkippedFrames,
				RenderTotalFrames:      stats.RenderTotalFrames,
				OutputSkippedFrames:    stats.OutputSkippedFrames,
				OutputTotalFrames:      stats.OutputTotalFrames,
			})
		}
	}
}
