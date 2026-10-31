package playoutClient

import (
	"testing"
	"time"

	ve "github.com/adanalife/tripbot/pkg/playout-events"
)

// playheadFrom is the half of Playhead that can be wrong without a NATS server
// noticing: it decides whether a report is worth trusting and how far to
// advance it. A stale report has to be refused rather than extrapolated —
// answering !location from a playout that stopped reporting an hour ago would
// be confidently wrong rather than usefully absent.
func TestPlayheadFrom(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	stamp := func(d time.Duration) string {
		return now.Add(-d).Format(time.RFC3339Nano)
	}

	tests := []struct {
		name    string
		ev      ve.LastPlayed
		wantPos time.Duration
		wantOK  bool
	}{
		{
			name:    "advances the report by its age",
			ev:      ve.LastPlayed{Envelope: ve.Envelope{EmittedAt: stamp(3 * time.Second)}, File: "clip.MP4", PositionMs: 90_000},
			wantPos: 93 * time.Second,
			wantOK:  true,
		},
		{
			name:   "refuses a report older than the staleness bound",
			ev:     ve.LastPlayed{Envelope: ve.Envelope{EmittedAt: stamp(lastPlayedStaleAfter + time.Second)}, File: "clip.MP4", PositionMs: 90_000},
			wantOK: false,
		},
		{
			name:   "refuses a report stamped in the future",
			ev:     ve.LastPlayed{Envelope: ve.Envelope{EmittedAt: stamp(-time.Minute)}, File: "clip.MP4"},
			wantOK: false,
		},
		{
			name:   "refuses an unparseable timestamp",
			ev:     ve.LastPlayed{Envelope: ve.Envelope{EmittedAt: "whenever"}, File: "clip.MP4"},
			wantOK: false,
		},
		{
			name:   "refuses a report naming no file",
			ev:     ve.LastPlayed{Envelope: ve.Envelope{EmittedAt: stamp(time.Second)}, PositionMs: 5_000},
			wantOK: false,
		},
		{
			// Messages published before position_ms existed decode to 0, which
			// is start-of-clip — a real answer, not a missing one.
			name:    "a zero position is start-of-clip, not an absent report",
			ev:      ve.LastPlayed{Envelope: ve.Envelope{EmittedAt: stamp(2 * time.Second)}, File: "clip.MP4"},
			wantPos: 2 * time.Second,
			wantOK:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, pos, ok := playheadFrom(tt.ev, now)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if file != tt.ev.File {
				t.Errorf("file = %q, want %q", file, tt.ev.File)
			}
			if pos != tt.wantPos {
				t.Errorf("pos = %v, want %v", pos, tt.wantPos)
			}
		})
	}
}
