package vlcServer

import (
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/vlc-server"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	libvlc "github.com/adrg/libvlc-go/v3"
)

// swapGapTracker measures the dead-air at a clip boundary: armed when the
// current Media ends (or a caller requests a different clip), observed when
// the next Media reaches Playing. libvlc delivers events on its own threads,
// so the timestamp is an atomic rather than a mutex-guarded field.
type swapGapTracker struct {
	armedAtNanos atomic.Int64 // 0 = disarmed
}

func (t *swapGapTracker) arm(now time.Time) { t.armedAtNanos.Store(now.UnixNano()) }

func (t *swapGapTracker) disarm() { t.armedAtNanos.Store(0) }

// observe returns the seconds since arm and disarms. ok is false when the
// tracker wasn't armed — e.g. a Playing event with no preceding boundary.
func (t *swapGapTracker) observe(now time.Time) (seconds float64, ok bool) {
	armed := t.armedAtNanos.Swap(0)
	if armed == 0 {
		return 0, false
	}
	return time.Duration(now.UnixNano() - armed).Seconds(), true
}

// initSwapGapEvents wires the libvlc player events that bracket a clip
// boundary: EndReached arms the tracker, the next Playing records the gap
// as vlc_player_media_swap_gap_seconds. Manual clip changes arm in
// playAtIndex instead, since they never pass through EndReached.
//
// Callbacks only touch atomics, OTel, and slog — calling back into libvlc
// from an event callback can deadlock.
func (s *Server) initSwapGapEvents() error {
	em, err := s.Player.EventManager()
	if err != nil {
		return fmt.Errorf("fetching player event manager: %w", err)
	}
	if _, err := em.Attach(libvlc.MediaPlayerEndReached, func(libvlc.Event, interface{}) {
		s.swapGap.arm(time.Now())
	}, nil); err != nil {
		return fmt.Errorf("attaching EndReached handler: %w", err)
	}
	gapMetric := instrumentation.NewVLCSwapGap(c.Conf.Platform)
	if _, err := em.Attach(libvlc.MediaPlayerPlaying, func(libvlc.Event, interface{}) {
		if gap, ok := s.swapGap.observe(time.Now()); ok {
			gapMetric.Record(gap)
			slog.Debug("media swap gap", "gap_seconds", gap)
		}
	}, nil); err != nil {
		return fmt.Errorf("attaching Playing handler: %w", err)
	}
	return nil
}
