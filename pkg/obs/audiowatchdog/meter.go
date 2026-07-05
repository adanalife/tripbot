package audiowatchdog

import (
	"context"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/adanalife/tripbot/pkg/obs"
	goobs "github.com/andreykaipov/goobs"
	"github.com/andreykaipov/goobs/api/events"
	"github.com/andreykaipov/goobs/api/events/subscriptions"
)

// silenceFloorDB is the dBFS value reported for a multiplier of 0 (true
// silence) and the floor we clamp to. -60 dBFS is well below any audible bed
// level, so anything at or near it reads as "no audio."
const silenceFloorDB = -60.0

// meterReconnectDelay bounds how fast the meter reconnects after the OBS
// WebSocket drops, so a flapping OBS can't spin a tight reconnect loop.
const meterReconnectDelay = 5 * time.Second

// VolumeMeter holds the latest peak output level for one OBS input, fed by a
// long-lived OBS WebSocket connection subscribed to the InputVolumeMeters
// high-volume event (one frame every ~50ms). It exists because OBS exposes
// audio levels only as a pushed event — there is no request to poll them — so
// detecting "playing but silent" needs a persistent subscription. Read the
// current value with Level(); the connection self-heals on drop.
type VolumeMeter struct {
	inputName  string
	staleAfter time.Duration

	mu       sync.RWMutex
	lastDB   float64
	lastSeen time.Time
}

// NewVolumeMeter builds a meter for the named OBS input. staleAfter is how
// long a sample stays trusted — past it, Level reports fresh=false so callers
// fall back to other signals (the source may have stopped emitting meters
// entirely). It does not connect until Run is called.
func NewVolumeMeter(inputName string, staleAfter time.Duration) *VolumeMeter {
	return &VolumeMeter{
		inputName:  inputName,
		staleAfter: staleAfter,
		lastDB:     silenceFloorDB,
	}
}

// Level returns the most recent peak level in dBFS (floored at -60) and
// whether that sample is fresh enough to trust. fresh is false before the
// first sample and once a sample ages past staleAfter.
func (m *VolumeMeter) Level() (db float64, fresh bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.lastSeen.IsZero() || time.Since(m.lastSeen) > m.staleAfter {
		return m.lastDB, false
	}
	return m.lastDB, true
}

// Run maintains the OBS WebSocket connection and updates the latest level
// until ctx is cancelled. On any disconnect it waits meterReconnectDelay and
// reconnects. Intended to run as its own goroutine.
func (m *VolumeMeter) Run(ctx context.Context) {
	slog.InfoContext(ctx, "obs volume meter started", "input", m.inputName)
	for {
		if ctx.Err() != nil {
			return
		}
		m.connectAndConsume(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(meterReconnectDelay):
		}
	}
}

// connectAndConsume opens one subscribed connection and drains its event
// stream until the connection drops or ctx is cancelled. Returns so Run can
// reconnect.
func (m *VolumeMeter) connectAndConsume(ctx context.Context) {
	client, err := obs.Dial(ctx, goobs.WithEventSubscriptions(subscriptions.InputVolumeMeters))
	if err != nil {
		slog.WarnContext(ctx, "obs volume meter: connect failed", "err", err)
		return
	}
	defer func() {
		if err := client.Disconnect(); err != nil {
			slog.WarnContext(ctx, "obs volume meter: disconnect", "err", err)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-client.IncomingEvents:
			if !ok {
				// Channel closed — the connection dropped. Let Run reconnect.
				slog.WarnContext(ctx, "obs volume meter: event stream closed")
				return
			}
			meters, ok := ev.(*events.InputVolumeMeters)
			if !ok {
				continue
			}
			m.consume(meters)
		}
	}
}

// consume extracts our input's peak level from one InputVolumeMeters frame and
// records it. Frames that don't mention our input are ignored (so a momentary
// absence doesn't clobber the last good reading — staleness handles a
// sustained absence).
func (m *VolumeMeter) consume(meters *events.InputVolumeMeters) {
	for _, in := range meters.Inputs {
		if in == nil || in.Name != m.inputName {
			continue
		}
		db := peakDB(in.Levels)
		m.mu.Lock()
		m.lastDB = db
		m.lastSeen = time.Now()
		m.mu.Unlock()
		return
	}
}

// peakDB converts OBS's per-channel level multipliers to a single peak dBFS
// value. Each channel is [magnitude, peak, peakHold]; we take the loudest
// channel's peak (index 1) and convert to dB, clamped to the silence floor.
func peakDB(levels [][3]float64) float64 {
	maxMul := 0.0
	for _, ch := range levels {
		if ch[1] > maxMul {
			maxMul = ch[1]
		}
	}
	if maxMul <= 0 {
		return silenceFloorDB
	}
	db := 20 * math.Log10(maxMul)
	if db < silenceFloorDB {
		return silenceFloorDB
	}
	return db
}
