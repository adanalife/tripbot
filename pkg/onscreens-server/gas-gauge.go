package onscreensServer

import (
	"encoding/json"
	"fmt"
	"html"
	"log/slog"

	oe "github.com/adanalife/tripbot/pkg/onscreens-events"
	"github.com/nats-io/nats.go"
)

// gasGaugeLowFraction is the level under which the gauge draws as low fuel.
const gasGaugeLowFraction = 0.15

// renderGasGauge builds the gauge's HTML: an E-to-F bar filled to fraction
// (clamped to 0..1) with an optional caption. The caption comes off the wire,
// so it is escaped here.
func renderGasGauge(fraction float64, label string) string {
	fraction = min(max(fraction, 0), 1)
	class := "gas"
	if fraction < gasGaugeLowFraction {
		class = "gas low"
	}
	out := fmt.Sprintf(
		`<div class="%s"><span class="gas-e">E</span><div class="gas-track"><div class="gas-fill" style="width:%.0f%%"></div></div><span>F</span>`,
		class, fraction*100,
	)
	if label != "" {
		out += `<span class="gas-label">` + html.EscapeString(label) + `</span>`
	}
	return out + `</div>`
}

// handleGaugeShow sets the gas gauge's level and leaves it up until a
// gauge.hide. Lenient about the level (clamped), strict about the body.
// shortcut: the level lives in memory, so a restart drops the gauge until the
// next gauge.show; persist it like the middle text once something real feeds it.
func (s *Server) handleGaugeShow(m *nats.Msg) {
	var ev oe.GaugeShow
	if err := json.Unmarshal(m.Data, &ev); err != nil {
		slog.Error("nats: decode gauge.show", "err", err, "subject", m.Subject)
		return
	}
	s.GasGauge.Show(renderGasGauge(ev.Fraction, ev.Label))
}

func (s *Server) handleGaugeHide(_ *nats.Msg) { s.GasGauge.Hide() }
