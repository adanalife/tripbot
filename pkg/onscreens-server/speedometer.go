package onscreensServer

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	oe "github.com/adanalife/tripbot/pkg/onscreens-events"
	"github.com/nats-io/nats.go"
)

// speedometerTTL is how long a speed reading stays up without a fresh one.
// tripbot republishes every ten seconds, so this outlasts two missed updates
// and hides a speedometer whose feed stopped rather than freezing its last
// number on screen.
const speedometerTTL = 30 * time.Second

// newSpeedometer constructs the speedometer *Onscreen. Its expiry sweep runs
// every second so a stopped feed hides close to speedometerTTL, not up to a
// full default interval past it.
func newSpeedometer() *Onscreen {
	slog.Info("creating onscreen", "kind", "speedometer")
	return newOnscreen(time.Second)
}

// handleSpeedUpdate shows the van's speed, rounded to a whole mph, or hides
// the speedometer when the update carries no reading. Lenient: a malformed
// body is dropped and the last reading ages out on its own.
func (s *Server) handleSpeedUpdate(m *nats.Msg) {
	var ev oe.SpeedUpdate
	if err := json.Unmarshal(m.Data, &ev); err != nil {
		slog.Error("nats: decode speed.update", "err", err, "subject", m.Subject)
		return
	}
	if ev.MPH == nil {
		s.Speedometer.Hide()
		return
	}
	s.Speedometer.Refresh(fmt.Sprintf("%.0f", *ev.MPH), speedometerTTL)
}
