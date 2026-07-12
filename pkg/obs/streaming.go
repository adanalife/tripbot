// Package obs provides polling integration with the OBS WebSocket API.
package obs

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/adanalife/tripbot/pkg/instrumentation"
	goobs "github.com/andreykaipov/goobs"
)

// PollStreamingActive connects to the OBS WebSocket and updates the
// obs_streaming_active gauge every interval, stamping the series with the
// given streaming platform. Intended to be run as a long-lived goroutine.
// Reconnects automatically on connection loss.
func PollStreamingActive(ctx context.Context, platform string, interval time.Duration) {
	addr := os.Getenv("OBS_WEBSOCKET_ADDR")
	if addr == "" {
		addr = defaultOBSWebsocketAddr
	}
	passwd := os.Getenv("OBS_WEBSOCKET_PASSWD")
	if passwd == "" {
		passwd = "adanalife"
	}

	obsStats := instrumentation.NewOBSStats(platform)
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		poll(ctx, obsStats, addr, passwd, interval)
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
func poll(ctx context.Context, obsStats instrumentation.OBSStats, addr, passwd string, interval time.Duration) {
	client, err := goobs.New(addr, goobs.WithPassword(passwd))
	if err != nil {
		slog.ErrorContext(ctx, "obs websocket connect failed", "addr", addr, "err", err)
		obsStats.SetStreaming(false)
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
				// The outer loop reconnects after 10s and obs_streaming_active
				// is the alertable signal — keep this off Sentry.
				slog.WarnContext(ctx, "obs GetStreamStatus error", "err", err)
				obsStats.SetStreaming(false)
				return // trigger reconnect
			}
			obsStats.SetStreaming(resp.OutputActive)
			obsStats.UpdateStream(instrumentation.OBSStreamSnapshot{
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
			obsStats.Update(instrumentation.OBSStatsSnapshot{
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
