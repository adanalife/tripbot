package watchdog

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeDeps drives the watchdog with scripted (obs, twitch) responses,
// counts restarts, and signals when the script is exhausted so the test
// can cleanly tear down the loop. OBSActive advances the script (it's
// called every tick) and stashes the current pair; TwitchLive reads from
// that stash. Holds the final pair after exhaustion to avoid racing
// shutdown.
type fakeDeps struct {
	mu       sync.Mutex
	script   [][2]bool
	idx      int
	current  [2]bool
	restarts atomic.Int32
	doneCh   chan struct{}
	doneOnce sync.Once
}

func newFakeDeps(script [][2]bool) *fakeDeps {
	return &fakeDeps{script: script, doneCh: make(chan struct{})}
}

func (f *fakeDeps) deps() WatchdogDeps {
	return WatchdogDeps{
		OBSActive: func(context.Context) (bool, error) {
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
			return f.current[0], nil
		},
		TwitchLive: func(context.Context) (bool, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.current[1], nil
		},
		Restart: func(context.Context) error {
			f.restarts.Add(1)
			return nil
		},
	}
}

func runUntilExhausted(t *testing.T, deps *fakeDeps, threshold int, cooldown time.Duration) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go WatchSilentDisconnect(ctx, deps.deps(), 2*time.Millisecond, threshold, cooldown)
	select {
	case <-deps.doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("script not exhausted")
	}
	cancel()
	// Give the loop a beat to exit so racing restarts don't bleed past the assertion.
	time.Sleep(10 * time.Millisecond)
}

func TestWatchSilentDisconnect_FiresAfterThresholdMisses(t *testing.T) {
	deps := newFakeDeps([][2]bool{
		{true, false}, // miss 1
		{true, false}, // miss 2
		{true, false}, // miss 3 → restart (resets misses, sets lastRestart)
		{true, true},  // healthy
	})
	runUntilExhausted(t, deps, 3, time.Minute)
	if got := deps.restarts.Load(); got != 1 {
		t.Fatalf("restart count: want 1, got %d", got)
	}
}

func TestWatchSilentDisconnect_TransientMissDoesNotFire(t *testing.T) {
	deps := newFakeDeps([][2]bool{
		{true, false}, // miss 1
		{true, false}, // miss 2
		{true, true},  // recovered — reset
		{true, false}, // miss 1 (counter reset)
		{true, true},
	})
	runUntilExhausted(t, deps, 3, time.Minute)
	if got := deps.restarts.Load(); got != 0 {
		t.Fatalf("restart count: want 0, got %d", got)
	}
}

func TestWatchSilentDisconnect_CooldownSuppressesRapidRestarts(t *testing.T) {
	deps := newFakeDeps([][2]bool{
		{true, false}, {true, false}, {true, false}, // → restart 1
		{true, false}, {true, false}, {true, false}, // cooldown blocks
		{true, false}, {true, false}, {true, false},
	})
	runUntilExhausted(t, deps, 3, time.Hour)
	if got := deps.restarts.Load(); got != 1 {
		t.Fatalf("restart count: want 1 (cooldown suppresses retries), got %d", got)
	}
}

func TestWatchSilentDisconnect_ObsInactiveSkips(t *testing.T) {
	deps := newFakeDeps([][2]bool{
		{false, false}, {false, false}, {false, false}, {false, false},
	})
	runUntilExhausted(t, deps, 3, time.Minute)
	if got := deps.restarts.Load(); got != 0 {
		t.Fatalf("restart count: want 0 (OBS not streaming), got %d", got)
	}
}

func TestWatchSilentDisconnect_HelixErrorResetsMisses(t *testing.T) {
	var tickCount atomic.Int32
	var restarts atomic.Int32
	done := make(chan struct{})
	var doneOnce sync.Once

	deps := WatchdogDeps{
		OBSActive: func(context.Context) (bool, error) {
			return true, nil
		},
		TwitchLive: func(context.Context) (bool, error) {
			n := tickCount.Add(1)
			switch n {
			case 1, 2:
				return false, nil // miss 1, miss 2
			case 3:
				return false, errors.New("helix transient") // resets misses
			case 4, 5:
				return false, nil // miss 1, miss 2 (no third → no restart)
			default:
				doneOnce.Do(func() { close(done) })
				return true, nil
			}
		},
		Restart: func(context.Context) error {
			restarts.Add(1)
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go WatchSilentDisconnect(ctx, deps, 2*time.Millisecond, 3, time.Minute)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("test loop did not advance")
	}
	cancel()
	time.Sleep(10 * time.Millisecond)

	if got := restarts.Load(); got != 0 {
		t.Fatalf("restart count: want 0 (helix error reset misses before threshold), got %d", got)
	}
}
