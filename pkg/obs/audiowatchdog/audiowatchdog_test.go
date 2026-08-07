package audiowatchdog

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/obs"
	"github.com/adanalife/tripbot/pkg/obs/beds"
)

// tick is one scripted evaluation: what OBS reports for the source's media
// state, whether SomaFM probes reachable, and the meter's level + freshness.
type tick struct {
	state     string
	reachable bool
	db        float64
	fresh     bool
	// console, if set, runs before this tick is evaluated: an operator action
	// that repoints the source without telling the watchdog.
	console func(*fakeDeps)
}

// fakeDeps drives Watch with a scripted sequence of ticks and counts the swaps
// in each direction. The script advances on Level, the one hook called on every
// tick unconditionally; MediaState and SomaFMReachable read the stashed current
// tick so all three reflect the same scripted evaluation. The probe is
// deliberately not the advancing hook — it only runs on ticks that can act on
// the answer, so driving the script from it would stall the moment the watchdog
// correctly declines to probe. Signals doneCh once exhausted so the test can
// tear the loop down cleanly, holding the last tick to avoid racing shutdown.
type fakeDeps struct {
	mu      sync.Mutex
	script  []tick
	idx     int
	current tick

	toFallback atomic.Int32
	toSomaFM   atomic.Int32
	advances   atomic.Int32
	probes     atomic.Int32

	// bed is the selected background-audio bed; the SomaFM outage machinery
	// only runs while it's SomaFM.
	bed beds.Bed
	// sourceLocal stands in for OBS: what the background-audio source is
	// actually pointed at. The swap hooks move it, so the watchdog reads back
	// the consequences of its own actions rather than a value the test asserts
	// into place.
	sourceLocal bool

	doneCh   chan struct{}
	doneOnce sync.Once
}

func newFakeDeps(script []tick) *fakeDeps {
	return &fakeDeps{script: script, bed: beds.SomaFM, doneCh: make(chan struct{})}
}

// onBed returns the same fake scripted to a different selected bed.
func (f *fakeDeps) onBed(b beds.Bed) *fakeDeps {
	f.bed = b
	return f
}

func (f *fakeDeps) deps() Deps {
	return Deps{
		SomaFMReachable: func(context.Context) bool {
			f.probes.Add(1)
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.current.reachable
		},
		Level: func() (float64, bool) {
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
				if f.current.console != nil {
					f.current.console(f)
				}
			}
			return f.current.db, f.current.fresh
		},
		MediaState: func(context.Context) (string, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.current.state, nil
		},
		SwapToFallback: func(context.Context) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.sourceLocal = true
			f.toFallback.Add(1)
			return nil
		},
		SwapToSomaFM: func(context.Context) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.sourceLocal = false
			f.toSomaFM.Add(1)
			return nil
		},
		SourceIsLocal: func(context.Context) (bool, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.sourceLocal, nil
		},
		ActiveBed: func() beds.Bed { return f.bed },
		AdvanceAlbum: func(context.Context) error {
			f.advances.Add(1)
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

func TestWatch_ConsoleReselectingSomaFMFallsBackAgain(t *testing.T) {
	// Mid-outage, an operator picks SomaFM again in the console. That repoints
	// the source at the dead stream without the watchdog swapping anything, so
	// the stream goes silent — and the watchdog has to notice and fall back a
	// second time rather than sit waiting for a recovery it already handled.
	// (2026-08-01: it sat, and Twitch was silent until SomaFM came back.)
	reselect := ended
	reselect.console = func(f *fakeDeps) { f.sourceLocal = false }
	deps := newFakeDeps([]tick{ended, ended, ended, reselect, ended, ended, ended})
	runUntilExhausted(t, deps, cfg(3, 4, 0))
	if got := deps.toFallback.Load(); got != 2 {
		t.Fatalf("to_fallback swaps: want 2, got %d", got)
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

func TestWatch_AdvancesAlbumWhenTrackEnds(t *testing.T) {
	// Album tracks play unlooped, so OBS reports ENDED between them — each one
	// is the cue to queue the next track.
	deps := newFakeDeps([]tick{playing, ended, playing, ended, playing}).onBed(beds.Album)
	runUntilExhausted(t, deps, cfg(3, 4, time.Minute))
	if got := deps.advances.Load(); got != 2 {
		t.Fatalf("album advances: want 2, got %d", got)
	}
}

func TestWatch_LocalBedNeverSwapsToFallback(t *testing.T) {
	// A run of down ticks that would trip the SomaFM fallback. On a local bed
	// there's nothing to fall back to — swapping would stomp the operator's
	// choice of bed — so the outage machinery must stay out of it.
	for _, bed := range []beds.Bed{beds.CarHum, beds.Album} {
		t.Run(string(bed), func(t *testing.T) {
			deps := newFakeDeps([]tick{ended, ended, ended, ended, ended}).onBed(bed)
			runUntilExhausted(t, deps, cfg(3, 4, 0))
			if got := deps.toFallback.Load(); got != 0 {
				t.Fatalf("to_fallback swaps on %s: want 0, got %d", bed, got)
			}
			if got := deps.toSomaFM.Load(); got != 0 {
				t.Fatalf("to_somafm swaps on %s: want 0, got %d", bed, got)
			}
		})
	}
}

func TestWatch_ProbesOnlyWhileStrandedOnFallback(t *testing.T) {
	// The probe opens a fresh connection to SomaFM's edge every time it runs, so
	// it must only run on a tick whose outcome depends on the answer: stranded on
	// the local bed, waiting to swap back. A local bed the operator picked, or a
	// healthy SomaFM stream, needs no probe — and probing anyway put every
	// platform's bot on the edge every interval, which is enough traffic from one
	// IP to get it firewalled (2026-08-02).
	t.Run("local bed never probes", func(t *testing.T) {
		for _, bed := range []beds.Bed{beds.CarHum, beds.Album} {
			deps := newFakeDeps([]tick{ended, ended, ended, playing}).onBed(bed)
			runUntilExhausted(t, deps, cfg(3, 4, 0))
			if got := deps.probes.Load(); got != 0 {
				t.Errorf("probes on %s bed: want 0, got %d", bed, got)
			}
		}
	})

	t.Run("healthy somafm stream never probes", func(t *testing.T) {
		deps := newFakeDeps([]tick{playing, playing, playing, playing})
		runUntilExhausted(t, deps, cfg(3, 4, 0))
		if got := deps.probes.Load(); got != 0 {
			t.Errorf("probes while the stream is healthy: want 0, got %d", got)
		}
	})

	t.Run("probes once stranded on the fallback", func(t *testing.T) {
		// 3 down ticks strand us on the local bed; only the ticks after that
		// swap consult the edge.
		deps := newFakeDeps([]tick{ended, ended, ended, ended, ended})
		runUntilExhausted(t, deps, cfg(3, 4, 0))
		if got := deps.probes.Load(); got == 0 {
			t.Fatal("no probe while stranded on the fallback: SomaFM can never be seen to recover")
		}
	})
}

func TestWatch_CarHumBedDoesNotAdvanceAlbum(t *testing.T) {
	// The car-hum drone loops forever; an ENDED tick there is a wedge, not a
	// track boundary, and must not walk an album playlist that isn't playing.
	deps := newFakeDeps([]tick{ended, ended, ended}).onBed(beds.CarHum)
	runUntilExhausted(t, deps, cfg(3, 4, time.Minute))
	if got := deps.advances.Load(); got != 0 {
		t.Fatalf("album advances on carhum: want 0, got %d", got)
	}
}
