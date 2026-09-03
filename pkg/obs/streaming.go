// Package obs provides integration with the OBS WebSocket API.
package obs

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/adanalife/tripbot/pkg/instrumentation"
	goobs "github.com/andreykaipov/goobs"
	"github.com/andreykaipov/goobs/api/events"
	"github.com/andreykaipov/goobs/api/events/subscriptions"
)

// PollStreamingActive connects to the OBS WebSocket and updates the
// obs_streaming_active gauge — immediately when OBS pushes a stream-state
// change, and every interval as a reconcile — stamping the series with the
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
	// failures counts consecutive dial failures, and is what backs the retry
	// off. A reached OBS resets it, so a genuine blip still reconnects in 10s.
	failures := 0
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		if poll(ctx, obsStats, addr, passwd, interval, failures) {
			failures = 0
		} else {
			failures++
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(reconnectWait(failures)):
		}
	}
}

// maxReconnectWait caps the dial backoff at ~5 minutes, reached after five
// consecutive failures. The ceiling is a recovery latency: an OBS coming back
// waits up to that long to be dialed, which is cheap against the alternative —
// a platform whose OBS is scaled to zero (the console's chat-only mode is
// exactly that shape) dials every 10s forever, and each attempt logs. That was
// measured at 99.7% of tripbot's error/warn feed, ~26k lines a day.
const maxReconnectWait = 5 * time.Minute

// reconnectWait is how long to wait before the next dial after n consecutive
// failures: 10s doubling to maxReconnectWait. n == 0 means the last connection
// succeeded and merely dropped, so it gets the base wait.
func reconnectWait(n int) time.Duration {
	wait := 10 * time.Second
	for range min(n, 6) {
		wait *= 2
		if wait >= maxReconnectWait {
			return maxReconnectWait
		}
	}
	return wait
}

// streamStateFromEvent reports the streaming-active flag carried by ev, and
// whether ev is a stream-state event at all. OutputActive is the same field
// GetStreamStatus returns — including staying true across an OBS-detected
// reconnect — so the pushed value and the polled value never disagree.
func streamStateFromEvent(ev any) (active, isStreamState bool) {
	e, ok := ev.(*events.StreamStateChanged)
	if !ok {
		return false, false
	}
	return e.OutputActive, true
}

// poll connects once and loops until the context is cancelled or the
// connection drops, publishing gauges off both the Outputs event stream and
// the interval tick. failures is how many consecutive dials have already
// failed, which decides how loudly another failure is logged. It reports
// whether the connection was established.
func poll(ctx context.Context, obsStats instrumentation.OBSStats, addr, passwd string, interval time.Duration, failures int) bool {
	client, err := goobs.New(addr,
		goobs.WithPassword(passwd),
		goobs.WithEventSubscriptions(subscriptions.Outputs))
	if err != nil {
		// A platform whose OBS deployment is scaled to zero fails here every
		// retry, forever. obs_streaming_active is the alertable signal — keep
		// this off Sentry, same as the in-loop failures below, and say it once
		// per outage rather than on every retry.
		level := slog.LevelWarn
		if failures > 0 {
			level = slog.LevelDebug
		}
		slog.Log(ctx, level, "obs websocket connect failed", "addr", addr, "err", err)
		obsStats.SetStreaming(false)
		return false
	}
	defer func() {
		if err := client.Disconnect(); err != nil {
			slog.WarnContext(ctx, "obs disconnect", "err", err)
		}
	}()

	slog.InfoContext(ctx, "obs websocket connected", "addr", addr)

	// The tick stays load-bearing alongside the event stream: a state change
	// that lands while the connection is down is never replayed, and
	// GetStreamStatus is also the only source for the stream-output gauges
	// (bytes, congestion, dropped frames), which OBS pushes no event for.
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return true
		case ev, ok := <-client.IncomingEvents:
			if !ok {
				// Channel closed — the connection dropped. The gauge going
				// stale is what alerts, so publish the known-bad value and
				// let the outer loop reconnect.
				slog.WarnContext(ctx, "obs event stream closed")
				obsStats.SetStreaming(false)
				return true
			}
			active, isStreamState := streamStateFromEvent(ev)
			if !isStreamState {
				continue
			}
			slog.InfoContext(ctx, "obs stream state changed", "active", active)
			obsStats.SetStreaming(active)
		case <-ticker.C:
			resp, err := client.Stream.GetStreamStatus()
			if err != nil {
				// Transient: OBS pod restart, network blip, websocket drop.
				// The outer loop reconnects after 10s and obs_streaming_active
				// is the alertable signal — keep this off Sentry.
				slog.WarnContext(ctx, "obs GetStreamStatus error", "err", err)
				obsStats.SetStreaming(false)
				return true // trigger reconnect
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
