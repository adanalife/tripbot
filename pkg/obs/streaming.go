// Package obs provides polling integration with the OBS WebSocket API.
package obs

import (
	"context"
	"log"
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
		log.Printf("obs: websocket connect to %s failed: %v", addr, err)
		instrumentation.OBSStreaming.Set(false)
		return
	}
	defer func() {
		if err := client.Disconnect(); err != nil {
			log.Printf("obs: disconnect: %v", err)
		}
	}()

	log.Printf("obs: connected to websocket at %s", addr)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			resp, err := client.Stream.GetStreamStatus()
			if err != nil {
				log.Printf("obs: GetStreamStatus error: %v", err)
				instrumentation.OBSStreaming.Set(false)
				return // trigger reconnect
			}
			instrumentation.OBSStreaming.Set(resp.OutputActive)
		}
	}
}
