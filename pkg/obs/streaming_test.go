package obs

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/andreykaipov/goobs/api/events"
)

// levelRecorder captures the highest slog level emitted.
type levelRecorder struct {
	slog.Handler
	max slog.Level
}

func (l *levelRecorder) Enabled(context.Context, slog.Level) bool { return true }

func (l *levelRecorder) Handle(_ context.Context, r slog.Record) error {
	if r.Level > l.max {
		l.max = r.Level
	}
	return nil
}

// A platform whose OBS deployment is scaled to zero fails to connect on every
// retry, forever. Logging that at error level bridges each attempt into Sentry
// and drains the monthly quota, so an unreachable OBS must stay at warn.
func TestPollConnectFailureStaysBelowError(t *testing.T) {
	rec := &levelRecorder{Handler: slog.Default().Handler(), max: slog.LevelDebug}
	prev := slog.Default()
	slog.SetDefault(slog.New(rec))
	defer slog.SetDefault(prev)

	// Port 1 on loopback refuses immediately — no listener, no timeout wait.
	if poll(context.Background(), instrumentation.NewOBSStats("test"), "127.0.0.1:1", "pw", time.Second, 0) {
		t.Fatal("poll reported a connection to a refused port")
	}

	if rec.max >= slog.LevelError {
		t.Fatalf("connect failure logged at %v; must stay below ERROR to keep it out of Sentry", rec.max)
	}
	if rec.max < slog.LevelWarn {
		t.Fatalf("connect failure logged at %v; expected a WARN so the failure is still visible", rec.max)
	}
}

// The Outputs subscription carries more than stream state, so the poller has
// to pick its event out of the stream and leave the gauge alone for the rest.
func TestStreamStateFromEvent(t *testing.T) {
	cases := []struct {
		name          string
		ev            any
		wantActive    bool
		wantIsChanged bool
	}{
		{"started", &events.StreamStateChanged{OutputActive: true, OutputState: "OBS_WEBSOCKET_OUTPUT_STARTED"}, true, true},
		{"stopped", &events.StreamStateChanged{OutputActive: false, OutputState: "OBS_WEBSOCKET_OUTPUT_STOPPED"}, false, true},
		// OBS knows it dropped and is retrying; the output is still active, so
		// the gauge must not flap. Catching the silent half-open is the
		// watchdog's job, not this gauge's.
		{"reconnecting", &events.StreamStateChanged{OutputActive: true, OutputState: "OBS_WEBSOCKET_OUTPUT_RECONNECTING"}, true, true},
		{"other event", &events.RecordStateChanged{OutputActive: true}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			active, isChanged := streamStateFromEvent(tc.ev)
			if isChanged != tc.wantIsChanged {
				t.Fatalf("isStreamState = %v, want %v", isChanged, tc.wantIsChanged)
			}
			if active != tc.wantActive {
				t.Fatalf("active = %v, want %v", active, tc.wantActive)
			}
		})
	}
}

// The dial backoff is what keeps a parked platform's OBS off the error feed: it
// retried every 10s forever and each attempt logged. A dropped connection still
// reconnects at the base wait, so a genuine blip is not penalised.
func TestReconnectWait(t *testing.T) {
	for _, tc := range []struct {
		failures int
		want     time.Duration
	}{
		{0, 10 * time.Second},
		{1, 20 * time.Second},
		{2, 40 * time.Second},
		{4, 160 * time.Second},
		{5, maxReconnectWait},
		{99, maxReconnectWait},
	} {
		if got := reconnectWait(tc.failures); got != tc.want {
			t.Errorf("reconnectWait(%d) = %v, want %v", tc.failures, got, tc.want)
		}
	}
}

// Repeat failures within one outage drop to DEBUG — the first is the one worth
// seeing, and a scaled-to-zero OBS otherwise emits an identical WARN forever.
func TestPollRepeatConnectFailureIsQuiet(t *testing.T) {
	rec := &levelRecorder{Handler: slog.Default().Handler(), max: slog.LevelDebug}
	prev := slog.Default()
	slog.SetDefault(slog.New(rec))
	defer slog.SetDefault(prev)

	poll(context.Background(), instrumentation.NewOBSStats("test"), "127.0.0.1:1", "pw", time.Second, 3)

	if rec.max >= slog.LevelWarn {
		t.Fatalf("repeat connect failure logged at %v; expected below WARN", rec.max)
	}
}
