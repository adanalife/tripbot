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

const capMaxAge = 47 * time.Hour

// capFixture answers the broadcast-cap loop's hooks from a staged OBS state
// and broadcast start time, and counts restarts.
type capFixture struct {
	mu       sync.Mutex
	state    obs.StreamState
	started  time.Time
	restarts int
	outcomes []error
	// restartErr is what Restart returns.
	restartErr error
}

func (f *capFixture) set(state obs.StreamState, started time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state, f.started = state, started
}

func (f *capFixture) restartCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.restarts
}

func (f *capFixture) deps() BroadcastCapDeps {
	return BroadcastCapDeps{
		Platform: "twitch",
		OBSState: func(context.Context) (obs.StreamState, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.state, nil
		},
		StartedAt: func(context.Context) (time.Time, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.started, nil
		},
		Restart: func(context.Context) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.restarts++
			return f.restartErr
		},
		OnRestart: func(_ context.Context, err error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.outcomes = append(f.outcomes, err)
		},
	}
}

// tick advances the bubble's clock by one interval and waits for the loop to
// finish that tick.
func tick(n int) {
	for range n {
		time.Sleep(watchInterval)
		synctest.Wait()
	}
}

// startCap runs the loop in the bubble and returns a stop func that cancels
// it and waits for it to return.
func startCap(t *testing.T, f *capFixture) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		WatchBroadcastCap(ctx, f.deps(), watchInterval, capMaxAge)
	}()
	synctest.Wait()
	return func() { cancel(); <-done }
}

// A broadcast under the cap is left alone; the tick after it crosses the cap
// restarts it, and only once.
func TestBroadcastCapRestartsOncePerBroadcast(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &capFixture{}
		// Three ticks short of the cap.
		f.set(obs.StreamSteady, time.Now().Add(-capMaxAge+3*watchInterval+watchInterval/2))
		stop := startCap(t, f)
		defer stop()

		tick(3)
		if got := f.restartCount(); got != 0 {
			t.Fatalf("restarts under the cap = %d, want 0", got)
		}
		tick(1)
		if got := f.restartCount(); got != 1 {
			t.Fatalf("restarts after crossing the cap = %d, want 1", got)
		}
		// The platform kept the same broadcast: restarting again would only add
		// gaps before the same cut.
		tick(10)
		if got := f.restartCount(); got != 1 {
			t.Fatalf("restarts on a kept broadcast = %d, want 1", got)
		}
		if len(f.outcomes) != 1 || f.outcomes[0] != nil {
			t.Fatalf("OnRestart calls = %v, want one nil", f.outcomes)
		}
	})
}

// A failed restart is reported to OnRestart with its error, and still counts
// as this broadcast's one restart: the loop does not retry it every tick.
func TestBroadcastCapReportsAFailedRestartOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		restartErr := errors.New("obs: connection refused")
		f := &capFixture{restartErr: restartErr}
		f.set(obs.StreamSteady, time.Now().Add(-capMaxAge-time.Minute))
		stop := startCap(t, f)
		defer stop()

		tick(5)
		if got := f.restartCount(); got != 1 {
			t.Fatalf("restarts after a failed restart = %d, want 1", got)
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.outcomes) != 1 || !errors.Is(f.outcomes[0], restartErr) {
			t.Fatalf("OnRestart calls = %v, want one carrying %v", f.outcomes, restartErr)
		}
	})
}

// A new broadcast gets its own restart when it reaches the cap in turn.
func TestBroadcastCapRearmsOnNewBroadcast(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &capFixture{}
		f.set(obs.StreamSteady, time.Now().Add(-capMaxAge-time.Minute))
		stop := startCap(t, f)
		defer stop()

		tick(1)
		f.set(obs.StreamSteady, time.Now().Add(-capMaxAge-time.Second))
		tick(1)
		if got := f.restartCount(); got != 2 {
			t.Fatalf("restarts across two capped broadcasts = %d, want 2", got)
		}
	})
}

// Only a steady output is this loop's to restart, and a zero start time is an
// unknown, not an ancient broadcast.
func TestBroadcastCapLeavesOtherStatesAlone(t *testing.T) {
	old := func() time.Time { return time.Now().Add(-capMaxAge - time.Hour) }
	cases := []struct {
		name    string
		state   obs.StreamState
		started func() time.Time
	}{
		{"output stopped", obs.StreamInactive, old},
		{"output reconnecting", obs.StreamReconnecting, old},
		{"start time unknown", obs.StreamSteady, func() time.Time { return time.Time{} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := &capFixture{}
				f.set(tc.state, tc.started())
				stop := startCap(t, f)
				defer stop()
				tick(5)
				if got := f.restartCount(); got != 0 {
					t.Fatalf("restarts = %d, want 0", got)
				}
			})
		})
	}
}

// The wait ends on the first offline reading, and a failed read keeps
// waiting rather than counting as offline.
func TestAwaitOffline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		answers := []error{errors.New("gateway blip"), nil}
		calls := 0
		live := func(context.Context) (bool, error) {
			defer func() { calls++ }()
			if calls < len(answers) && answers[calls] != nil {
				return false, answers[calls]
			}
			return calls < 3, nil
		}
		start := time.Now()
		if err := awaitOffline(t.Context(), live, time.Hour, time.Second); err != nil {
			t.Fatal(err)
		}
		if calls != 4 || time.Since(start) != 4*time.Second {
			t.Fatalf("calls = %d after %v, want 4 after 4s", calls, time.Since(start))
		}
	})
}

// A platform that never reports offline ends the wait at the timeout, with an
// error the caller logs before starting the output anyway.
func TestAwaitOfflineTimesOut(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		live := func(context.Context) (bool, error) { return true, nil }
		start := time.Now()
		if err := awaitOffline(t.Context(), live, time.Minute, 10*time.Second); err == nil {
			t.Fatal("awaitOffline = nil, want a timeout error")
		}
		if got := time.Since(start); got != time.Minute {
			t.Fatalf("waited %v, want 1m", got)
		}
	})
}

func TestRetryStart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		flaky := func(context.Context) error {
			calls++
			if calls < 2 {
				return errors.New("obs: connection refused")
			}
			return nil
		}
		if err := retryStart(t.Context(), flaky, 3, time.Second); err != nil || calls != 2 {
			t.Fatalf("retryStart = %v after %d calls, want nil after 2", err, calls)
		}

		calls = 0
		broken := func(context.Context) error { calls++; return errors.New("obs: OutputRunning") }
		if err := retryStart(t.Context(), broken, 3, time.Second); err == nil || calls != 3 {
			t.Fatalf("retryStart = %v after %d calls, want an error after 3", err, calls)
		}
	})
}
