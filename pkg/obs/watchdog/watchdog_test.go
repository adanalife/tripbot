package watchdog

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeDeps drives the watchdog with scripted (obs, live) responses,
// counts restarts, and signals when the script is exhausted so the test
// can cleanly tear down the loop. OBSActive advances the script (it's
// called every tick) and stashes the current pair; ChannelLive reads from
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
		ChannelLive: func(context.Context) (bool, error) {
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

// A recovery that held, followed by an unrelated outage, recovers on detection
// instead of waiting out the cooldown — the shape of the 2026-07-29 TikTok ban,
// where the room ran fine for 21 minutes between two independent deaths.
func TestWatchSilentDisconnect_HeldRecoveryRetiresCooldown(t *testing.T) {
	deps := newFakeDeps([][2]bool{
		{true, false}, {true, false}, {true, false}, // → restart 1
		{true, true}, {true, true}, {true, true}, // recovery holds for threshold ticks
		{true, false}, {true, false}, {true, false}, // → restart 2, cooldown retired
	})
	runUntilExhausted(t, deps, 3, time.Hour)
	if got := deps.restarts.Load(); got != 2 {
		t.Fatalf("restart count: want 2 (held recovery retires the cooldown), got %d", got)
	}
}

// A recovery too brief to prove itself leaves the cooldown in force, so a
// platform flapping live/offline still can't drive a restart loop.
func TestWatchSilentDisconnect_BriefRecoveryKeepsCooldown(t *testing.T) {
	deps := newFakeDeps([][2]bool{
		{true, false}, {true, false}, {true, false}, // → restart 1
		{true, true},                                // one live tick: not enough to prove the recovery took
		{true, false}, {true, false}, {true, false}, // cooldown still blocks
	})
	runUntilExhausted(t, deps, 3, time.Hour)
	if got := deps.restarts.Load(); got != 1 {
		t.Fatalf("restart count: want 1 (brief recovery keeps the cooldown), got %d", got)
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

// liveAnswer is one scripted ChannelLive result: a definite live/offline
// answer, or an error standing for "unknown" (TikTok's IsLive returning
// ErrIPBlockedOrBanned under throttling is the real case).
type liveAnswer struct {
	live bool
	err  error
}

// runLiveScript drives the watchdog through script with OBS always streaming,
// and reports how many restarts fired. interval is explicit because the misses
// held across an unknown answer age out after threshold*interval.
func runLiveScript(t *testing.T, script []liveAnswer, interval time.Duration, threshold int) int32 {
	t.Helper()
	var idx, restarts atomic.Int32
	done := make(chan struct{})
	var doneOnce sync.Once

	deps := WatchdogDeps{
		OBSActive: func(context.Context) (bool, error) { return true, nil },
		ChannelLive: func(context.Context) (bool, error) {
			n := int(idx.Add(1)) - 1
			if n >= len(script) {
				doneOnce.Do(func() { close(done) })
				return true, nil
			}
			return script[n].live, script[n].err
		},
		Restart: func(context.Context) error { restarts.Add(1); return nil },
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go WatchSilentDisconnect(ctx, deps, interval, threshold, time.Minute)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("script not exhausted")
	}
	cancel()
	time.Sleep(10 * time.Millisecond)
	return restarts.Load()
}

// An unknown live status must not clear the misses already collected: a check
// that fails every other tick would otherwise hold the count under the
// threshold forever, leaving a dark channel unrecovered.
func TestWatchSilentDisconnect_LiveCheckErrorHoldsMisses(t *testing.T) {
	unknown := liveAnswer{err: errors.New("live-check transient")}
	offline := liveAnswer{}
	got := runLiveScript(t, []liveAnswer{offline, offline, unknown, offline}, 10*time.Millisecond, 3)
	if got != 1 {
		t.Fatalf("restart count: want 1 (misses survive an unknown answer), got %d", got)
	}
}

// Held misses still expire: once the last definite answer is older than it
// takes to declare a death, the evidence is discarded rather than firing a
// restart off observations from another era.
func TestWatchSilentDisconnect_HeldMissesGoStale(t *testing.T) {
	unknown := liveAnswer{err: errors.New("live-check transient")}
	offline := liveAnswer{}
	script := []liveAnswer{offline, offline}
	for i := 0; i < 8; i++ { // 80ms of unknowns against a 30ms staleness window
		script = append(script, unknown)
	}
	script = append(script, offline, offline)
	if got := runLiveScript(t, script, 10*time.Millisecond, 3); got != 0 {
		t.Fatalf("restart count: want 0 (stale misses discarded), got %d", got)
	}
}
