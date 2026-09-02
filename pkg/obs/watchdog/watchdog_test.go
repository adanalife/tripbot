package watchdog

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/adanalife/tripbot/pkg/obs"
)

// watchInterval is the tick the watchdog runs at under test. synctest puts the
// bubble on a fake clock, so the value is arbitrary and costs no wall time: a
// scripted run of any length finishes instantly, and every tick lands on an
// exact interval boundary instead of racing the real scheduler.
const watchInterval = time.Second

// step is one tick's worth of scripted answers from OBS and the platform,
// including how a restart fired on this tick resolves.
type step struct {
	obsState   obs.StreamState
	live       bool
	obsErr     error
	liveErr    error
	restartErr error
}

// The scripted answers the tests compose runs out of. A miss is the case the
// watchdog exists for: OBS happily pushing at a channel nobody can watch.
var (
	miss      = step{obsState: obs.StreamSteady}
	healthy   = step{obsState: obs.StreamSteady, live: true}
	obsIdle   = step{obsState: obs.StreamInactive}
	unknown   = step{obsState: obs.StreamSteady, liveErr: errors.New("live-check transient")}
	obsDown   = step{obsErr: errors.New("obs websocket unreachable")}
	missNoFix = step{obsState: obs.StreamSteady, restartErr: errors.New("StartStream: OutputRunning (500)")}
	// OBS knows the session failed and is retrying, with the channel dark —
	// the 2026-08-28 state. Distinct from `miss` only in what OBS admits.
	reconnecting = step{obsState: obs.StreamReconnecting}
	// A reconnect that landed: OBS still reports reconnecting, but the channel
	// is back. Recovery working is exactly what the grace protects.
	reconnectingLive = step{obsState: obs.StreamReconnecting, live: true}
)

// graceTicks is how many ticks of the test interval fit in reconnectGrace — the
// stand-down the watchdog owes OBS's own retry before it intervenes.
var graceTicks = int(reconnectGrace / watchInterval)

// repeat is n copies of one step, for scripting spans measured in the
// production-scale reconnectGrace rather than in handfuls of ticks.
func repeat(s step, n int) []step {
	out := make([]step, n)
	for i := range out {
		out[i] = s
	}
	return out
}

// fixture answers the watchdog's hooks from the currently-staged step and
// counts restart attempts — a restart the staged step fails still counts, which
// is what the cooldown tests below measure. The mutex is load-bearing even
// inside a synctest bubble:
// the watchdog still runs on its own goroutine, so -race wants the shared
// state guarded.
type fixture struct {
	mu       sync.Mutex
	cur      step
	restarts int
	// outcomes collects one entry per OnRestart call (the restart's error, nil
	// on success); recoveries counts OnRecovered calls. Both hooks are always
	// wired here, so every scripted run also exercises the transition
	// callbacks.
	outcomes   []error
	recoveries int
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

func (f *fixture) hookCalls() ([]error, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]error(nil), f.outcomes...), f.recoveries
}

func (f *fixture) deps() WatchdogDeps {
	return WatchdogDeps{
		OBSState: func(context.Context) (obs.StreamState, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.cur.obsState, f.cur.obsErr
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
			return f.cur.restartErr
		},
		OnRestart: func(_ context.Context, restartErr error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.outcomes = append(f.outcomes, restartErr)
		},
		OnRecovered: func(context.Context) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.recoveries++
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
	return runFixture(t, script, threshold, cooldown).restartCount()
}

// runFixture is run, returning the whole fixture so tests can also assert on
// the OnRestart / OnRecovered hook calls the script produced. Safe to read
// after the bubble closes: the loop has exited and nothing else holds it.
func runFixture(t *testing.T, script []step, threshold int, cooldown time.Duration) *fixture {
	t.Helper()
	return runFixtureOn(t, "", script, threshold, cooldown)
}

// runFixtureOn is runFixture with the deps' Platform set, for the tests that
// assert on the per-platform metric label.
func runFixtureOn(t *testing.T, platform string, script []step, threshold int, cooldown time.Duration) *fixture {
	t.Helper()
	f := &fixture{}
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel() // lets the loop exit before the bubble closes

		deps := f.deps()
		deps.Platform = platform
		go WatchSilentDisconnect(ctx, deps, watchInterval, threshold, cooldown)
		synctest.Wait() // wait out the loop's startup, up to its first ticker receive

		for _, s := range script {
			f.stage(s)
			time.Sleep(watchInterval)
			synctest.Wait()
		}
	})
	return f
}

// The output reports active for several polls before the teardown completes —
// the span the old fixed pause guessed at and a half-open socket overran.
func TestAwaitOutputStopped_WaitsOutTheTeardown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		polls := 0
		err := awaitOutputStopped(t.Context(), func(context.Context) (bool, error) {
			polls++
			return polls < 4, nil
		})
		if err != nil {
			t.Fatalf("awaitOutputStopped: %v", err)
		}
		if polls != 4 {
			t.Fatalf("status polls: want 4 (three active, then stopped), got %d", polls)
		}
	})
}

// An output that never goes down fails the restart rather than issuing a
// StartStream that OBS would reject with OutputRunning.
func TestAwaitOutputStopped_TimesOutWhileStillActive(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		err := awaitOutputStopped(t.Context(), func(context.Context) (bool, error) {
			return true, nil
		})
		if err == nil {
			t.Fatal("awaitOutputStopped: want an error when the output never stops")
		}
	})
}

// An unreachable OBS says nothing about the output, so the restart aborts
// instead of starting a second output on top of one that may still be up.
func TestAwaitOutputStopped_StatusErrorAborts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		want := errors.New("obs websocket unreachable")
		err := awaitOutputStopped(t.Context(), func(context.Context) (bool, error) {
			return false, want
		})
		if !errors.Is(err, want) {
			t.Fatalf("awaitOutputStopped: want %v, got %v", want, err)
		}
	})
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

// A restart that fails arms the cooldown just like one that succeeds. Stamping
// the attempt only past the error check left a failing recovery with no
// timestamp to measure the cooldown against, so it re-fired on every single
// tick — which is how one OBS teardown that outlived the restart's settle pause
// became 24 rejected StartStreams in 9 hours, each one re-stopping an output
// already mid-teardown.
func TestWatchSilentDisconnect_FailedRestartArmsCooldown(t *testing.T) {
	script := []step{
		missNoFix, missNoFix, missNoFix, // → attempt 1, fails
		missNoFix, missNoFix, missNoFix, // cooldown blocks the retry
		missNoFix, missNoFix, missNoFix,
	}
	if got := run(t, script, 3, time.Hour); got != 1 {
		t.Fatalf("restart attempts: want 1 (a failed restart arms the cooldown), got %d", got)
	}
}

// The cooldown rate-limits a failing recovery rather than abandoning it: the
// channel is still dark, so once the timer elapses it gets another attempt.
func TestWatchSilentDisconnect_FailedRestartRetriesAfterCooldown(t *testing.T) {
	script := make([]step, 0, 8)
	for range 8 {
		script = append(script, missNoFix)
	}
	// Threshold 3 puts the first attempt on tick 3; a 3-tick cooldown puts the
	// second on tick 6, leaving ticks 7-8 suppressed again.
	if got := run(t, script, 3, 3*watchInterval); got != 2 {
		t.Fatalf("restart attempts: want 2 (cooldown rate-limits, it doesn't abandon), got %d", got)
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

// Each forced restart reports its outcome through OnRestart — nil for a
// restart that completed, the Restart error for one that didn't. This is the
// seam cmd/tripbot records watchdog_restart events through.
func TestWatchSilentDisconnect_OnRestartReportsOutcome(t *testing.T) {
	f := runFixture(t, []step{miss, miss, miss, healthy}, 3, time.Minute)
	outcomes, _ := f.hookCalls()
	if len(outcomes) != 1 || outcomes[0] != nil {
		t.Fatalf("outcomes = %v, want one nil (successful restart)", outcomes)
	}

	f = runFixture(t, []step{missNoFix, missNoFix, missNoFix}, 3, time.Hour)
	outcomes, _ = f.hookCalls()
	if len(outcomes) != 1 || outcomes[0] == nil {
		t.Fatalf("outcomes = %v, want one non-nil (failed restart)", outcomes)
	}
}

// OnRecovered fires exactly when the cooldown retires: a restart happened and
// the channel then held live for threshold ticks.
func TestWatchSilentDisconnect_OnRecoveredFiresWhenRecoveryHolds(t *testing.T) {
	f := runFixture(t, []step{miss, miss, miss, healthy, healthy, healthy}, 3, time.Hour)
	if _, recoveries := f.hookCalls(); recoveries != 1 {
		t.Fatalf("recoveries = %d, want 1 (held recovery)", recoveries)
	}
}

// A live streak with no preceding restart is ordinary health, not a recovery.
func TestWatchSilentDisconnect_OnRecoveredNeedsARestart(t *testing.T) {
	f := runFixture(t, []step{healthy, healthy, healthy, healthy}, 3, time.Minute)
	if _, recoveries := f.hookCalls(); recoveries != 0 {
		t.Fatalf("recoveries = %d, want 0 (nothing was recovered from)", recoveries)
	}
}

// A recovery too brief to prove itself reports nothing.
func TestWatchSilentDisconnect_OnRecoveredSkipsBriefRecovery(t *testing.T) {
	f := runFixture(t, []step{miss, miss, miss, healthy, miss, miss}, 3, time.Hour)
	if _, recoveries := f.hookCalls(); recoveries != 0 {
		t.Fatalf("recoveries = %d, want 0 (recovery never held)", recoveries)
	}
}

// The hooks are optional: a deps with neither set (the pre-wiring shape every
// caller outside cmd/tripbot has) must run the full detect-and-restart path
// without panicking.
func TestWatchSilentDisconnect_NilHooks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fixture{}
		deps := f.deps()
		deps.OnRestart = nil
		deps.OnRecovered = nil
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go WatchSilentDisconnect(ctx, deps, watchInterval, 3, time.Hour)
		synctest.Wait()

		for _, s := range []step{miss, miss, miss, healthy, healthy, healthy} {
			f.stage(s)
			time.Sleep(watchInterval)
			synctest.Wait()
		}
		if f.restartCount() != 1 {
			t.Fatalf("restart count: want 1, got %d", f.restartCount())
		}
	})
}

// The 2026-08-28 outage in miniature: OBS's RTMP session dies, OBS announces a
// reconnect and then stops trying, and the channel stays dark. Before the
// grace, a reconnecting output zeroed the miss counter on every tick, so no
// amount of elapsed time ever reached the threshold and the stream stayed dark
// until a human intervened.
func TestReconnectingOutputIsRecoveredOnceTheGraceExpires(t *testing.T) {
	script := append(repeat(reconnecting, graceTicks), repeat(reconnecting, 3)...)
	if got := run(t, script, 3, time.Minute); got != 1 {
		t.Errorf("restarts = %d, want 1 — a wedged reconnect must be recovered", got)
	}
}

// The other half of the same bargain: OBS's own reconnect is left alone while
// it still has a chance to land. A watchdog that forced a Stop+Start into the
// first seconds of a retry would race the recovery it is waiting for.
func TestReconnectingOutputIsLeftAloneInsideTheGrace(t *testing.T) {
	if got := run(t, repeat(reconnecting, graceTicks-1), 3, time.Minute); got != 0 {
		t.Errorf("restarts = %d, want 0 — OBS's own reconnect owns this window", got)
	}
}

// A reconnect that works ends the matter: the channel comes back, the live
// answer clears the misses, and the grace never matters. Sitting in
// reconnecting for hours is fine as long as the stream is watchable.
func TestReconnectingWithTheChannelLiveNeverRestarts(t *testing.T) {
	script := repeat(reconnectingLive, graceTicks*2)
	if got := run(t, script, 3, time.Minute); got != 0 {
		t.Errorf("restarts = %d, want 0 — a landed reconnect is not a fault", got)
	}
}

// Misses collected before OBS admits the failure are held, not forgotten. The
// old boolean zeroed them, so a divergence that decayed into a reconnect reset
// all the evidence for it — the outage got *quieter* the worse it became.
func TestMissesSurviveAReconnectAppearingMidDivergence(t *testing.T) {
	// Two misses (threshold 3, so no restart yet), then the full grace of
	// reconnecting, then one more miss to cross the line.
	script := append([]step{miss, miss}, repeat(reconnecting, graceTicks)...)
	script = append(script, miss)
	if got := run(t, script, 3, time.Minute); got != 1 {
		t.Errorf("restarts = %d, want 1 — misses must survive a reconnect", got)
	}
}

// An inactive output still stands down unconditionally, which is the case
// folding the two into one boolean was protecting. A scaled-to-0 or
// operator-stopped instance is not an outage and must never be restarted.
func TestInactiveOutputStillStandsDownForever(t *testing.T) {
	if got := run(t, repeat(obsIdle, graceTicks*2), 3, time.Minute); got != 0 {
		t.Errorf("restarts = %d, want 0 — a stopped output is not a fault", got)
	}
}

// The grace is measured per reconnect, not cumulatively: an output that
// recovers and later fails again gets the full stand-down the second time.
func TestGraceRestartsWithEachNewReconnect(t *testing.T) {
	script := repeat(reconnecting, graceTicks-1)
	script = append(script, healthy)
	script = append(script, repeat(reconnecting, graceTicks-1)...)
	if got := run(t, script, 3, time.Minute); got != 0 {
		t.Errorf("restarts = %d, want 0 — each reconnect earns its own grace", got)
	}
}

// A recovery that mechanically succeeds but never holds — the 2026-08-23/24
// YouTube shape, where the fault was in the broadcast and every bounce of the
// output was a no-op — gets maxRecoveryRounds attempts and then none: the
// watchdog stands down rather than churning the connection for hours.
func TestWatchSilentDisconnect_StandsDownAfterMaxRounds(t *testing.T) {
	// Threshold 3 and a 3-tick cooldown put one attempt every three ticks, so
	// the cap lands on tick 9 and a script four times that long fires nothing
	// more; without the cap it would fire twelve.
	script := repeat(miss, 12*3)
	if got := run(t, script, 3, 3*watchInterval); got != maxRecoveryRounds {
		t.Fatalf("restart count: want %d (stands down at the cap), got %d", maxRecoveryRounds, got)
	}
}

// A recovery that holds resets the run, so a platform that dies and is
// recovered many times over a long stream never reaches the cap.
func TestWatchSilentDisconnect_HeldRecoveryResetsTheRounds(t *testing.T) {
	deaths := maxRecoveryRounds + 2
	var script []step
	for range deaths {
		script = append(script, miss, miss, miss, healthy, healthy, healthy)
	}
	if got := run(t, script, 3, time.Hour); got != deaths {
		t.Fatalf("restart count: want %d (a held recovery resets the rounds), got %d", deaths, got)
	}
}

// Stopping the output re-arms a stood-down watchdog: the next session is a
// fresh one and gets its recoveries back.
func TestWatchSilentDisconnect_InactiveOutputRearmsAfterStandDown(t *testing.T) {
	script := append(repeat(miss, 12), obsIdle, miss, miss, miss)
	if got := run(t, script, 3, 3*watchInterval); got != maxRecoveryRounds+1 {
		t.Fatalf("restart count: want %d (a stopped output re-arms the watchdog), got %d", maxRecoveryRounds+1, got)
	}
}
