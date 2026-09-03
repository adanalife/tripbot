package audiowatchdog

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/adanalife/tripbot/pkg/obs"
	"github.com/adanalife/tripbot/pkg/obs/beds"
)

// interval is the evaluation period the watchdog runs at under test. synctest
// puts the bubble on a fake clock, so the value is arbitrary and costs no wall
// time: a scripted run of any length finishes instantly, and every tick lands
// on an exact interval boundary instead of racing the real scheduler.
const interval = time.Second

// tick is one scripted evaluation: what OBS reports for the source's media
// state, whether SomaFM probes reachable, and the meter's level + freshness.
type tick struct {
	state     string
	reachable bool
	db        float64
	fresh     bool
	// console, if set, runs as this tick is staged: an operator action that
	// repoints the source without telling the watchdog.
	console func(*fakeDeps)
	// obsDown makes MediaState fail, as it does while OBS is restarting.
	obsDown bool
}

// fakeDeps answers the watchdog's hooks from the currently-staged tick and
// counts the swaps in each direction. The driver stages one tick per interval,
// so all three of Level, MediaState and SomaFMReachable reflect the same
// scripted evaluation. The mutex is load-bearing even inside a synctest
// bubble: the watchdog still runs on its own goroutine, so -race wants the
// shared state guarded.
type fakeDeps struct {
	mu      sync.Mutex
	current tick

	toFallback atomic.Int32
	toSomaFM   atomic.Int32
	advances   atomic.Int32
	probes     atomic.Int32
	resyncs    atomic.Int32

	// bed is the selected background-audio bed; the SomaFM outage machinery
	// only runs while it's SomaFM.
	bed beds.Bed
	// sourceLocal stands in for OBS: what the background-audio source is
	// actually pointed at. The swap hooks move it, so the watchdog reads back
	// the consequences of its own actions rather than a value the test asserts
	// into place.
	sourceLocal bool
}

func (f *fakeDeps) stage(tk tick) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.current = tk
	if tk.console != nil {
		tk.console(f)
	}
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
			return f.current.db, f.current.fresh
		},
		MediaState: func(context.Context) (string, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.current.obsDown {
				return "", errors.New("obs websocket unreachable")
			}
			return f.current.state, nil
		},
		Resync: func(context.Context) { f.resyncs.Add(1) },
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

// run drives the watchdog through exactly one evaluation per scripted tick and
// returns the fake to assert on. Each tick is staged before the clock advances,
// so the evaluation that fires sees that tick and no other; synctest.Wait then
// blocks until the watchdog has finished processing it and is parked back on
// the ticker. That makes the whole run deterministic — no exhaustion
// signalling, no bailout timeout, and no drain window for a late swap to slip
// through. Safe to read after the bubble closes: the loop has exited and
// nothing else holds the fake.
func run(t *testing.T, script []tick, cfg Config) *fakeDeps {
	t.Helper()
	return runOnBed(t, beds.SomaFM, script, cfg)
}

// runOnBed is run with a different selected bed.
func runOnBed(t *testing.T, bed beds.Bed, script []tick, cfg Config) *fakeDeps {
	t.Helper()
	f := &fakeDeps{bed: bed}
	cfg.Interval = interval
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel() // lets the loop exit before the bubble closes

		go Watch(ctx, "twitch", f.deps(), cfg)
		synctest.Wait() // wait out the loop's startup, up to its first ticker receive

		for _, tk := range script {
			f.stage(tk)
			time.Sleep(interval)
			synctest.Wait()
		}
	})
	return f
}

// playing is a healthy tick (source PLAYING, SomaFM up, audible).
var playing = tick{state: obs.MediaStatePlaying, reachable: true, db: -18, fresh: true}

// ended is a wedged tick (EOF, SomaFM down, no fresh level).
var ended = tick{state: obs.MediaStateEnded, reachable: false, db: silenceFloorDB, fresh: false}

func cfg(fail, recover int, cooldown time.Duration) Config {
	return Config{FailThreshold: fail, RecoverThreshold: recover, SilenceDB: -50, Cooldown: cooldown}
}

func TestWatch_FallsBackAfterThresholdDownTicks(t *testing.T) {
	deps := run(t, []tick{ended, ended, ended, playing, playing}, cfg(3, 4, time.Minute))
	if got := deps.toFallback.Load(); got != 1 {
		t.Fatalf("to_fallback swaps: want 1, got %d", got)
	}
}

func TestWatch_TransientDownDoesNotFallBack(t *testing.T) {
	deps := run(t, []tick{ended, ended, playing, ended, playing, playing}, cfg(3, 4, time.Minute))
	if got := deps.toFallback.Load(); got != 0 {
		t.Fatalf("to_fallback swaps: want 0, got %d", got)
	}
}

func TestWatch_SilenceWhilePlayingTriggersFallback(t *testing.T) {
	// Source reports PLAYING but the meter shows sustained silence — the
	// "playing but silent" case the audio meter exists to catch.
	silent := tick{state: obs.MediaStatePlaying, reachable: false, db: -58, fresh: true}
	deps := run(t, []tick{silent, silent, silent, playing, playing}, cfg(3, 4, time.Minute))
	if got := deps.toFallback.Load(); got != 1 {
		t.Fatalf("to_fallback swaps: want 1, got %d", got)
	}
}

func TestWatch_StaleSilenceDoesNotTrigger(t *testing.T) {
	// Low level but not fresh (meter connection stale) + source PLAYING — not
	// enough to fall back; we don't act on an untrusted level.
	staleSilent := tick{state: obs.MediaStatePlaying, reachable: true, db: -58, fresh: false}
	deps := run(t, []tick{staleSilent, staleSilent, staleSilent, staleSilent}, cfg(3, 4, time.Minute))
	if got := deps.toFallback.Load(); got != 0 {
		t.Fatalf("to_fallback swaps: want 0, got %d", got)
	}
}

func TestWatch_SwapsBackAfterSomaFMRecovers(t *testing.T) {
	// 3 down ticks → fallback, then 4 reachable ticks → swap back. Cooldown 0
	// so the second swap isn't suppressed.
	up := tick{state: obs.MediaStatePlaying, reachable: true, db: -18, fresh: true}
	deps := run(t, []tick{ended, ended, ended, up, up, up, up, up}, cfg(3, 4, 0))
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
	deps := run(t, []tick{ended, ended, ended, reselect, ended, ended, ended}, cfg(3, 4, 0))
	if got := deps.toFallback.Load(); got != 2 {
		t.Fatalf("to_fallback swaps: want 2, got %d", got)
	}
}

func TestWatch_CooldownSuppressesSwapBack(t *testing.T) {
	// Fall back, then SomaFM is immediately reachable — but the cooldown from
	// the fallback swap blocks the swap-back within the window.
	up := tick{state: obs.MediaStatePlaying, reachable: true, db: -18, fresh: true}
	deps := run(t, []tick{ended, ended, ended, up, up, up, up, up}, cfg(3, 4, time.Hour))
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
	deps := runOnBed(t, beds.Album, []tick{playing, ended, playing, ended, playing}, cfg(3, 4, time.Minute))
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
			deps := runOnBed(t, bed, []tick{ended, ended, ended, ended, ended}, cfg(3, 4, 0))
			if got := deps.toFallback.Load(); got != 0 {
				t.Fatalf("to_fallback swaps on %s: want 0, got %d", bed, got)
			}
			if got := deps.toSomaFM.Load(); got != 0 {
				t.Fatalf("to_somafm swaps on %s: want 0, got %d", bed, got)
			}
		})
	}
}

func TestWatch_ResyncsBedWhenOBSComesBack(t *testing.T) {
	// OBS restarts mid-stream and boots onto its own default bed. The Store's
	// remembered bed is now wrong, so the watchdog re-reads it once OBS answers
	// again — once at start, once after the outage, not on every tick.
	down := tick{obsDown: true}
	deps := run(t, []tick{playing, playing, down, down, playing, playing}, cfg(3, 4, time.Minute))
	if got := deps.resyncs.Load(); got != 2 {
		t.Fatalf("bed resyncs: want 2, got %d", got)
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
			deps := runOnBed(t, bed, []tick{ended, ended, ended, playing}, cfg(3, 4, 0))
			if got := deps.probes.Load(); got != 0 {
				t.Errorf("probes on %s bed: want 0, got %d", bed, got)
			}
		}
	})

	t.Run("healthy somafm stream never probes", func(t *testing.T) {
		deps := run(t, []tick{playing, playing, playing, playing}, cfg(3, 4, 0))
		if got := deps.probes.Load(); got != 0 {
			t.Errorf("probes while the stream is healthy: want 0, got %d", got)
		}
	})

	t.Run("probes once stranded on the fallback", func(t *testing.T) {
		// 3 down ticks strand us on the local bed; only the ticks after that
		// swap consult the edge.
		deps := run(t, []tick{ended, ended, ended, ended, ended}, cfg(3, 4, 0))
		if got := deps.probes.Load(); got == 0 {
			t.Fatal("no probe while stranded on the fallback: SomaFM can never be seen to recover")
		}
	})
}

// The probe backs off as an outage wears on. Without it a fallback that never
// ends is a probe that never stops — ~12k connections a day to one edge, from
// the one state whose whole problem may be that the edge has firewalled us.
func TestWatch_ProbeBacksOffWhileTheOutageLasts(t *testing.T) {
	const ticks = 60 // 3 to strand us, 57 stranded
	script := make([]tick, ticks)
	for i := range script {
		script[i] = ended
	}
	deps := run(t, script, cfg(3, 4, time.Minute))

	// Three ticks strand us, so 57 stranded ticks remain. The probe fires on
	// the first of them and then sits out 1, 3, 7, 15 and 31 ticks between
	// attempts, which lands exactly five probes inside the window — one per
	// tick would be 57.
	const wantProbes = 5
	if got := deps.probes.Load(); got != wantProbes {
		t.Fatalf("probes over %d stranded ticks: want %d, got %d", ticks-3, wantProbes, got)
	}
}

// A backed-off probe that finally answers must not make the swap back wait out
// another four of its own intervals — the edge is up and the stream should
// follow it within a few ticks.
func TestWatch_RecoveryIsPromptAfterABackedOffOutage(t *testing.T) {
	up := tick{state: obs.MediaStatePlaying, reachable: true, db: -18, fresh: true}
	script := make([]tick, 0, 40)
	for range 30 {
		script = append(script, ended)
	}
	for range 10 {
		script = append(script, up)
	}
	deps := run(t, script, cfg(3, 4, 0))
	if got := deps.toSomaFM.Load(); got != 1 {
		t.Fatalf("to_somafm swaps: want 1, got %d — a backed-off probe stranded the stream past the recovery", got)
	}
}

func TestWatch_AdvancesTheFallbackAlbumWhileStranded(t *testing.T) {
	// Stranded on the fallback, the album plays unlooped just as the selected
	// bed does, so ENDED ticks are track boundaries. The meter's playback-ended
	// subscription normally advances first; this is the backstop for when it is
	// down, which is likeliest exactly when OBS is unwell enough to strand us
	// here. Without it the outage ends in the silence the fallback exists to
	// prevent. The long cooldown keeps the run on the fallback bed.
	deps := run(t, []tick{ended, ended, ended, ended, ended}, cfg(3, 4, time.Hour))
	if got := deps.toFallback.Load(); got != 1 {
		t.Fatalf("to_fallback swaps: want 1, got %d", got)
	}
	if deps.advances.Load() == 0 {
		t.Fatal("no advance while stranded on the fallback album: it plays one track and falls silent")
	}
}

func TestWatch_CarHumBedDoesNotAdvanceAlbum(t *testing.T) {
	// The car-hum drone loops forever; an ENDED tick there is a wedge, not a
	// track boundary, and must not walk an album playlist that isn't playing.
	deps := runOnBed(t, beds.CarHum, []tick{ended, ended, ended}, cfg(3, 4, time.Minute))
	if got := deps.advances.Load(); got != 0 {
		t.Fatalf("album advances on carhum: want 0, got %d", got)
	}
}
