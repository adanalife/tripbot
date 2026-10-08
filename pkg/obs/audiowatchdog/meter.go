package audiowatchdog

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/adanalife/tripbot/pkg/obs"
	goobs "github.com/andreykaipov/goobs"
	"github.com/andreykaipov/goobs/api/events"
	"github.com/andreykaipov/goobs/api/events/subscriptions"
	"github.com/andreykaipov/goobs/api/requests/inputs"
	"github.com/andreykaipov/goobs/api/requests/mediainputs"
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
//
// The connection is also the watchdog's request channel: MediaInputState and
// InputSettings ride it, so the every-7s probes cost no connect + auth
// handshake of their own.
type VolumeMeter struct {
	inputName  string
	staleAfter time.Duration
	onEnded    func(context.Context) error

	mu       sync.RWMutex
	lastDB   float64
	lastSeen time.Time

	// clientMu guards client, which is nil whenever the meter is between
	// connections. Separate from mu so a request in flight can't hold up the
	// level updates arriving on the same connection.
	clientMu sync.RWMutex
	client   *goobs.Client
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
	m.setClient(client)
	defer func() {
		m.setClient(nil)
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

// setClient records the connection the meter is currently consuming, or nil
// while it has none.
func (m *VolumeMeter) setClient(client *goobs.Client) {
	m.clientMu.Lock()
	m.client = client
	m.clientMu.Unlock()
}

// heldClient returns the connection the meter is consuming. With no connection
// it returns ErrUnreachable wrapped, matching what pkg/obs's per-call helpers
// return when the dial fails — the meter is disconnected exactly when OBS is
// unreachable, so callers keep telling "OBS is down" apart from "the source
// isn't playing."
func (m *VolumeMeter) heldClient() (*goobs.Client, error) {
	m.clientMu.RLock()
	defer m.clientMu.RUnlock()
	if m.client == nil {
		return nil, fmt.Errorf("%w: volume meter not connected", obs.ErrUnreachable)
	}
	return m.client, nil
}

// MediaInputState returns the OBS media state string for a media-source input
// (e.g. "OBS_MEDIA_STATE_PLAYING") over the meter's held connection. The
// same answer obs.GetMediaInputState gives, without a connection of its own —
// which matters because the watchdog asks every tick, all day.
func (m *VolumeMeter) MediaInputState(_ context.Context, inputName string) (string, error) {
	client, err := m.heldClient()
	if err != nil {
		return "", err
	}
	resp, err := client.MediaInputs.GetMediaInputStatus(
		mediainputs.NewGetMediaInputStatusParams().WithInputName(inputName),
	)
	if err != nil {
		return "", err
	}
	return resp.MediaState, nil
}

// InputSettings reads an input's current settings over the meter's held
// connection. The request form of obs.GetInputSettings, on the same terms as
// MediaInputState.
func (m *VolumeMeter) InputSettings(_ context.Context, inputName string) (map[string]any, error) {
	client, err := m.heldClient()
	if err != nil {
		return nil, err
	}
	resp, err := client.Inputs.GetInputSettings(
		inputs.NewGetInputSettingsParams().WithInputName(inputName),
	)
	if err != nil {
		return nil, err
	}
	return resp.InputSettings, nil
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
