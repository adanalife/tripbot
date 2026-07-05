package audiowatchdog

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/obs"
)

// tick is one scripted evaluation: what OBS reports for the source's media
// state, whether SomaFM probes reachable, and the meter's level + freshness.
type tick struct {
	state     string
	reachable bool
	db        float64
	fresh     bool
}

// fakeDeps drives Watch with a scripted sequence of ticks and counts the swaps
// in each direction. The script advances on SomaFMReachable (the first hook
// called each tick); Level and MediaState read the stashed current tick so all
// three reflect the same scripted evaluation. Signals doneCh once exhausted so
// the test can tear the loop down cleanly, holding the last tick to avoid
// racing shutdown.
type fakeDeps struct {
	mu      sync.Mutex
	script  []tick
	idx     int
	current tick

	toFallback atomic.Int32
	toSomaFM   atomic.Int32

	doneCh   chan struct{}
	doneOnce sync.Once
}

func newFakeDeps(script []tick) *fakeDeps {
	return &fakeDeps{script: script, doneCh: make(chan struct{})}
}

func (f *fakeDeps) deps() Deps {
	return Deps{
		SomaFMReachable: func(context.Context) bool {
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.idx >= len(f.script) {
				f.doneOnce.Do(func() { close(f.doneCh) })
				if len(f.script) > 0 {
					f.current = f.script[len(f.script)-1]
				}
			} else {
				f.current = f.script[f.idx]
				f.idx++
			}
			return f.current.reachable
		},
		Level: func() (float64, bool) {
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.current.db, f.current.fresh
		},
		MediaState: func(context.Context) (string, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.current.state, nil
		},
		SwapToFallback: func(context.Context) error {
			f.toFallback.Add(1)
			return nil
		},
		SwapToSomaFM: func(context.Context) error {
			f.toSomaFM.Add(1)
			return nil
		},
	}
}

func runUntilExhausted(t *testing.T, deps *fakeDeps, cfg Config) {
	t.Helper()
	cfg.Interval = 2 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Watch(ctx, deps.deps(), cfg)
	select {
	case <-deps.doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("script not exhausted")
	}
	cancel()
	// Let the loop exit so a racing swap doesn't bleed past the assertion.
	time.Sleep(10 * time.Millisecond)
}

// playing is a healthy tick (source PLAYING, SomaFM up, audible).
var playing = tick{state: obs.MediaStatePlaying, reachable: true, db: -18, fresh: true}

// ended is a wedged tick (EOF, SomaFM down, no fresh level).
var ended = tick{state: obs.MediaStateEnded, reachable: false, db: silenceFloorDB, fresh: false}

func cfg(fail, recover int, cooldown time.Duration) Config {
	return Config{FailThreshold: fail, RecoverThreshold: recover, SilenceDB: -50, Cooldown: cooldown}
}

func TestWatch_FallsBackAfterThresholdDownTicks(t *testing.T) {
	deps := newFakeDeps([]tick{ended, ended, ended, playing, playing})
	runUntilExhausted(t, deps, cfg(3, 4, time.Minute))
	if got := deps.toFallback.Load(); got != 1 {
		t.Fatalf("to_fallback swaps: want 1, got %d", got)
	}
}

func TestWatch_TransientDownDoesNotFallBack(t *testing.T) {
	deps := newFakeDeps([]tick{ended, ended, playing, ended, playing, playing})
	runUntilExhausted(t, deps, cfg(3, 4, time.Minute))
	if got := deps.toFallback.Load(); got != 0 {
		t.Fatalf("to_fallback swaps: want 0, got %d", got)
	}
}

func TestWatch_SilenceWhilePlayingTriggersFallback(t *testing.T) {
	// Source reports PLAYING but the meter shows sustained silence — the
	// "playing but silent" case the audio meter exists to catch.
	silent := tick{state: obs.MediaStatePlaying, reachable: false, db: -58, fresh: true}
	deps := newFakeDeps([]tick{silent, silent, silent, playing, playing})
	runUntilExhausted(t, deps, cfg(3, 4, time.Minute))
	if got := deps.toFallback.Load(); got != 1 {
		t.Fatalf("to_fallback swaps: want 1, got %d", got)
	}
}

func TestWatch_StaleSilenceDoesNotTrigger(t *testing.T) {
	// Low level but not fresh (meter connection stale) + source PLAYING — not
	// enough to fall back; we don't act on an untrusted level.
	staleSilent := tick{state: obs.MediaStatePlaying, reachable: true, db: -58, fresh: false}
	deps := newFakeDeps([]tick{staleSilent, staleSilent, staleSilent, staleSilent})
	runUntilExhausted(t, deps, cfg(3, 4, time.Minute))
	if got := deps.toFallback.Load(); got != 0 {
		t.Fatalf("to_fallback swaps: want 0, got %d", got)
	}
}

func TestWatch_SwapsBackAfterSomaFMRecovers(t *testing.T) {
	// 3 down ticks → fallback, then 4 reachable ticks → swap back. Cooldown 0
	// so the second swap isn't suppressed.
	up := tick{state: obs.MediaStatePlaying, reachable: true, db: -18, fresh: true}
	deps := newFakeDeps([]tick{ended, ended, ended, up, up, up, up, up})
	runUntilExhausted(t, deps, cfg(3, 4, 0))
	if got := deps.toFallback.Load(); got != 1 {
		t.Fatalf("to_fallback swaps: want 1, got %d", got)
	}
	if got := deps.toSomaFM.Load(); got != 1 {
		t.Fatalf("to_somafm swaps: want 1, got %d", got)
	}
}

func TestWatch_CooldownSuppressesSwapBack(t *testing.T) {
	// Fall back, then SomaFM is immediately reachable — but the cooldown from
	// the fallback swap blocks the swap-back within the window.
	up := tick{state: obs.MediaStatePlaying, reachable: true, db: -18, fresh: true}
	deps := newFakeDeps([]tick{ended, ended, ended, up, up, up, up, up})
	runUntilExhausted(t, deps, cfg(3, 4, time.Hour))
	if got := deps.toFallback.Load(); got != 1 {
		t.Fatalf("to_fallback swaps: want 1, got %d", got)
	}
	if got := deps.toSomaFM.Load(); got != 0 {
		t.Fatalf("to_somafm swaps: want 0 (cooldown suppresses), got %d", got)
	}
}
