package audiowatchdog

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/andreykaipov/goobs/api/events"
	"github.com/andreykaipov/goobs/api/typedefs"
)

// The scene's other media sources raise MediaInputPlaybackEnded too — the
// dashcam player ends a clip every few minutes — so acting on one that isn't
// ours would advance the album on somebody else's track boundary.
func TestHandle_PlaybackEndedOnlyFiresForOurInput(t *testing.T) {
	var fired int
	m := NewVolumeMeter("Background Audio", time.Minute, func(context.Context) error {
		fired++
		return nil
	})
	for _, ev := range []any{
		&events.MediaInputPlaybackEnded{InputName: "Dashcam"},
		&events.MediaInputPlaybackEnded{InputName: "Background Audio"},
	} {
		m.handle(context.Background(), ev)
	}
	if fired != 1 {
		t.Errorf("handler fired %d times, want 1 (ours only)", fired)
	}
}

// A failing handler is logged, not fatal: the next track boundary tries again,
// and the level updates this connection also carries must keep flowing.
func TestHandle_SurvivesAFailingHandler(t *testing.T) {
	m := NewVolumeMeter("Background Audio", time.Minute, func(context.Context) error {
		return errors.New("obs unreachable")
	})
	m.handle(context.Background(), &events.MediaInputPlaybackEnded{InputName: "Background Audio"})

	m.handle(context.Background(), &events.InputVolumeMeters{
		Inputs: []*typedefs.InputVolumeMeter{{Name: "Background Audio", Levels: [][3]float64{{0.5, 1.0, 1.0}}}},
	})
	if db, fresh := m.Level(); !fresh || db != 0 {
		t.Errorf("level after a failed advance: got %v (fresh=%v), want 0 dB fresh", db, fresh)
	}
}

func TestPeakDB(t *testing.T) {
	cases := []struct {
		name   string
		levels [][3]float64
		want   float64
	}{
		{"no channels is silence", nil, silenceFloorDB},
		{"zero multiplier is silence", [][3]float64{{0, 0, 0}}, silenceFloorDB},
		{"unity peak is 0 dB", [][3]float64{{0.5, 1.0, 1.0}}, 0},
		{"half peak is ~-6 dB", [][3]float64{{0.1, 0.5, 0.5}}, -6.0206},
		{"takes the loudest channel's peak", [][3]float64{{0, 0.25, 0.25}, {0, 1.0, 1.0}}, 0},
		{"sub-floor multiplier clamps to floor", [][3]float64{{0, 0.0001, 0.0001}}, silenceFloorDB},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := peakDB(c.levels)
			if math.Abs(got-c.want) > 0.01 {
				t.Fatalf("peakDB(%v) = %v, want %v", c.levels, got, c.want)
			}
		})
	}
}
