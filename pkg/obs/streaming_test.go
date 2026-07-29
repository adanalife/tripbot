package obs

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/instrumentation"
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
	poll(context.Background(), instrumentation.NewOBSStats("test"), "127.0.0.1:1", "pw", time.Second)

	if rec.max >= slog.LevelError {
		t.Fatalf("connect failure logged at %v; must stay below ERROR to keep it out of Sentry", rec.max)
	}
	if rec.max < slog.LevelWarn {
		t.Fatalf("connect failure logged at %v; expected a WARN so the failure is still visible", rec.max)
	}
}
