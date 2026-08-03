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

// VolumeMeter watches one OBS input over a long-lived WebSocket connection: it
// holds the input's latest peak output level, and it relays the input's
// playback-ended events to onEnded. Both jobs need a subscription rather than a
// request — OBS exposes audio levels only as a pushed event (one frame every
// ~50ms), and the end of a track is a moment, not a state you can poll without
// leaving dead air behind. Read the current level with Level(); the connection
// self-heals on drop.
type VolumeMeter struct {
	inputName  string
	staleAfter time.Duration
	onEnded    func(context.Context) error

	mu       sync.RWMutex
	lastDB   float64
	lastSeen time.Time
}

// NewVolumeMeter builds a meter for the named OBS input. staleAfter is how
// long a sample stays trusted — past it, Level reports fresh=false so callers
// fall back to other signals (the source may have stopped emitting meters
// entirely). onEnded runs each time that input finishes playing its media, and
// may be nil. It does not connect until Run is called.
func NewVolumeMeter(inputName string, staleAfter time.Duration, onEnded func(context.Context) error) *VolumeMeter {
	return &VolumeMeter{
		inputName:  inputName,
		staleAfter: staleAfter,
		onEnded:    onEnded,
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
	client, err := obs.Dial(ctx, goobs.WithEventSubscriptions(
		subscriptions.InputVolumeMeters|subscriptions.MediaInputs))
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
			m.handle(ctx, ev)
		}
	}
}

// handle routes one event off the subscription; anything else on the stream is
// ignored.
//
// ponytail: onEnded runs inline. A wedged OBS can stall it, which stops the
// level updates and makes Level report fresh=false — a case the watchdog
// already treats as untrusted rather than silent. Dispatch it on its own
// goroutine if that stall ever costs more than the meter's freshness.
func (m *VolumeMeter) handle(ctx context.Context, ev any) {
	switch e := ev.(type) {
	case *events.InputVolumeMeters:
		m.consume(e)
	case *events.MediaInputPlaybackEnded:
		// Every media source in the scene raises this — the dashcam player ends
		// a clip every few minutes — so the input name is what makes it ours.
		if e.InputName != m.inputName || m.onEnded == nil {
			return
		}
		slog.InfoContext(ctx, "obs volume meter: input playback ended", "input", e.InputName)
		if err := m.onEnded(ctx); err != nil {
			slog.ErrorContext(ctx, "obs volume meter: playback-ended handler failed", "err", err)
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
