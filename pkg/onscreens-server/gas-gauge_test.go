package onscreensServer

import (
	"strings"
	"testing"

	"github.com/nats-io/nats.go"
)

func TestRenderGasGauge(t *testing.T) {
	cases := []struct {
		name     string
		fraction float64
		label    string
		want     []string
		absent   []string
	}{
		{"half", 0.5, "", []string{`class="gas"`, `width:50%`}, []string{"gas-label", "low"}},
		{"low fuel", 0.1, "", []string{`class="gas low"`, `width:10%`}, nil},
		{"over full clamps", 3, "", []string{`width:100%`}, nil},
		{"negative clamps", -1, "", []string{`width:0%`, `class="gas low"`}, nil},
		{"label is escaped", 0.5, "<b>12 gal</b>", []string{`&lt;b&gt;12 gal&lt;/b&gt;`}, []string{"<b>12"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderGasGauge(tc.fraction, tc.label)
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in %s", w, got)
				}
			}
			for _, a := range tc.absent {
				if strings.Contains(got, a) {
					t.Errorf("unexpected %q in %s", a, got)
				}
			}
		})
	}
}

func TestGaugeShowStaysUpUntilHidden(t *testing.T) {
	s := newTestServer(t)
	s.handleGaugeShow(&nats.Msg{Data: []byte(`{"emitted_at":"2026-01-01T00:00:00Z","fraction":0.4,"label":"8 gal"}`)})
	if !s.GasGauge.IsShowing() || !strings.Contains(s.GasGauge.Content(), "width:40%") {
		t.Fatalf("gauge not showing 40%%: showing=%v content=%s", s.GasGauge.IsShowing(), s.GasGauge.Content())
	}
	s.handleGaugeShow(&nats.Msg{Data: []byte(`not json`)})
	if !strings.Contains(s.GasGauge.Content(), "width:40%") {
		t.Errorf("a malformed show changed the gauge: %s", s.GasGauge.Content())
	}
	s.handleGaugeHide(&nats.Msg{})
	if s.GasGauge.IsShowing() {
		t.Error("gauge still showing after hide")
	}
}
