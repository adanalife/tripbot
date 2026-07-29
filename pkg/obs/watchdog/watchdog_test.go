package watchdog

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// watchInterval is the tick the watchdog runs at under test. synctest puts the
// bubble on a fake clock, so the value is arbitrary and costs no wall time: a
// scripted run of any length finishes instantly, and every tick lands on an
// exact interval boundary instead of racing the real scheduler.
const watchInterval = time.Second

// step is one tick's worth of scripted answers from OBS and the platform.
type step struct {
	obsActive bool
	live      bool
	obsErr    error
	liveErr   error
}

// The scripted answers the tests compose runs out of. A miss is the case the
// watchdog exists for: OBS happily pushing at a channel nobody can watch.
var (
	miss    = step{obsActive: true}
	healthy = step{obsActive: true, live: true}
	obsIdle = step{}
	unknown = step{obsActive: true, liveErr: errors.New("live-check transient")}
	obsDown = step{obsErr: errors.New("obs websocket unreachable")}
)

// fixture answers the watchdog's hooks from the currently-staged step and
// counts restarts. The mutex is load-bearing even inside a synctest bubble:
// the watchdog still runs on its own goroutine, so -race wants the shared
// state guarded.
type fixture struct {
	mu       sync.Mutex
	cur      step
	restarts int
}

func (f *fixture) stage(s step) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cur = s
}

func (f *fixture) restartCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.restarts
}

func (f *fixture) deps() WatchdogDeps {
	return WatchdogDeps{
		OBSActive: func(context.Context) (bool, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.cur.obsActive, f.cur.obsErr
		},
		ChannelLive: func(context.Context) (bool, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.cur.live, f.cur.liveErr
		},
		Restart: func(context.Context) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.restarts++
			return nil
		},
	}
}

// run drives the watchdog through exactly one tick per step and reports how
// many restarts fired. Each step is staged before the clock advances, so the
// tick that fires sees that step and no other; synctest.Wait then blocks until
// the watchdog has finished processing it and is parked back on the ticker.
// That makes the whole run deterministic — no exhaustion signalling, no bailout
// timeout, and no drain window for a late restart to slip through.
func run(t *testing.T, script []step, threshold int, cooldown time.Duration) int {
	t.Helper()
	var restarts int
	synctest.Test(t, func(t *testing.T) {
		f := &fixture{}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel() // lets the loop exit before the bubble closes

		go WatchSilentDisconnect(ctx, f.deps(), watchInterval, threshold, cooldown)
		synctest.Wait() // wait out the loop's startup, up to its first ticker receive

		for _, s := range script {
			f.stage(s)
			time.Sleep(watchInterval)
			synctest.Wait()
		}
		restarts = f.restartCount()
	})
	return restarts
}

func TestWatchSilentDisconnect_FiresAfterThresholdMisses(t *testing.T) {
	got := run(t, []step{miss, miss, miss, healthy}, 3, time.Minute)
	if got != 1 {
		t.Fatalf("restart count: want 1, got %d", got)
	}
}

func TestWatchSilentDisconnect_TransientMissDoesNotFire(t *testing.T) {
	// The recovery in the middle resets the counter, so neither run of misses
	// ever reaches the threshold.
	got := run(t, []step{miss, miss, healthy, miss, healthy}, 3, time.Minute)
	if got != 0 {
		t.Fatalf("restart count: want 0, got %d", got)
	}
}

func TestWatchSilentDisconnect_CooldownSuppressesRapidRestarts(t *testing.T) {
	script := []step{
		miss, miss, miss, // → restart 1
		miss, miss, miss, // cooldown blocks
		miss, miss, miss,
	}
	if got := run(t, script, 3, time.Hour); got != 1 {
		t.Fatalf("restart count: want 1 (cooldown suppresses retries), got %d", got)
	}
}

// A recovery that held, followed by an unrelated outage, recovers on detection
// instead of waiting out the cooldown — the shape of the 2026-07-29 TikTok ban,
// where the room ran fine for 21 minutes between two independent deaths.
func TestWatchSilentDisconnect_HeldRecoveryRetiresCooldown(t *testing.T) {
	script := []step{
		miss, miss, miss, // → restart 1
		healthy, healthy, healthy, // recovery holds for threshold ticks
		miss, miss, miss, // → restart 2, cooldown retired
	}
	if got := run(t, script, 3, time.Hour); got != 2 {
		t.Fatalf("restart count: want 2 (held recovery retires the cooldown), got %d", got)
	}
}

// A recovery too brief to prove itself leaves the cooldown in force, so a
// platform flapping live/offline still can't drive a restart loop.
func TestWatchSilentDisconnect_BriefRecoveryKeepsCooldown(t *testing.T) {
	script := []step{
		miss, miss, miss, // → restart 1
		healthy,          // one live tick: not enough to prove the recovery took
		miss, miss, miss, // cooldown still blocks
	}
	if got := run(t, script, 3, time.Hour); got != 1 {
		t.Fatalf("restart count: want 1 (brief recovery keeps the cooldown), got %d", got)
	}
}

func TestWatchSilentDisconnect_ObsInactiveSkips(t *testing.T) {
	got := run(t, []step{obsIdle, obsIdle, obsIdle, obsIdle}, 3, time.Minute)
	if got != 0 {
		t.Fatalf("restart count: want 0 (OBS not streaming), got %d", got)
	}
}

// An unreachable OBS says nothing about the channel, and the gauge behind
// PollStreamingActive is the alert signal for it — so the watchdog drops the
// misses it had rather than letting an OBS blip race a real platform-side drop
// into a restart.
func TestWatchSilentDisconnect_ObsErrorResetsMisses(t *testing.T) {
	// Without the reset these five ticks would reach the threshold and fire.
	got := run(t, []step{miss, miss, obsDown, miss, miss}, 3, time.Minute)
	if got != 0 {
		t.Fatalf("restart count: want 0 (an OBS error discards collected misses), got %d", got)
	}
}

// An unknown live status must not clear the misses already collected: a check
// that fails every other tick would otherwise hold the count under the
// threshold forever, leaving a dark channel unrecovered.
func TestWatchSilentDisconnect_LiveCheckErrorHoldsMisses(t *testing.T) {
	got := run(t, []step{miss, miss, unknown, miss}, 3, time.Minute)
	if got != 1 {
		t.Fatalf("restart count: want 1 (misses survive an unknown answer), got %d", got)
	}
}

// Held misses still expire: once the last definite answer is older than it
// takes to declare a death (threshold × interval), the evidence is discarded
// rather than firing a restart off observations from another era.
func TestWatchSilentDisconnect_HeldMissesGoStale(t *testing.T) {
	script := []step{miss, miss}
	// Five unknowns outlast the 3-tick staleness window, so the two misses go.
	for range 5 {
		script = append(script, unknown)
	}
	script = append(script, miss, miss)
	if got := run(t, script, 3, time.Minute); got != 0 {
		t.Fatalf("restart count: want 0 (stale misses discarded), got %d", got)
	}
}
