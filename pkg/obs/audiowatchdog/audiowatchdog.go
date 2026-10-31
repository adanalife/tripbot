// Package audiowatchdog keeps audible background music on the Twitch stream
// when the radio drops. The radio bed is an ffmpeg_source playing an
// internet-radio stream; the 2026-06-23 outage showed
// it can go silent for minutes with no self-heal — OBS wedges on a clean EOF
// and stops retrying, a single edge feeds a stalling stream, or the station's whole
// edge tier goes down. None of these recover without a manual nudge.
//
// The watchdog watches the source's media state + output level and, when audio
// is down past a debounce, swaps the source onto a local license-clean bed so
// the stream isn't silent: the album on the mounted share, or the Car Hum FLAC
// baked into the OBS image when the share has no tracks. An outage lasts hours,
// and a music stream degraded to music beats one degraded to a drone. It probes
// the station in the background and swaps back once it is serving bytes again.
// All of it is the OBS-side analogue of the silent-disconnect stream watchdog
// in pkg/obs/watchdog — same injectable-deps shape so the decision loop
// unit-tests without a real OBS WebSocket. cmd/tripbot is the sole consumer,
// and runs one per platform: every platform boots on the album fallback and
// selects the radio, so neither the fallback nor the album advance is
// Twitch-specific. Every series the loop
// writes carries the platform it was started for.
package audiowatchdog

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/adanalife/tripbot/pkg/obs"
	"github.com/adanalife/tripbot/pkg/obs/beds"
)

// Cross-repo contracts with the adanalife/obs repo's scene config
// (config/Tripbot.json.tmpl) and Dockerfiles:
const (
	// BackgroundAudioInputName is the OBS source name of the music bed. Must
	// match the source "name" in the scene config. Every bed plays through this
	// one source, so the watchdog reads the same input whichever is selected.
	BackgroundAudioInputName = beds.InputName

	// radioProbeUserAgent overrides Go's default User-Agent on the probe.
	// Some Icecast servers reject "Go-http-client/1.1" outright — the
	// connection is closed before any response (the probe saw a bare EOF), so
	// the probe always reported unreachable and the stream could never swap
	// back off the fallback bed. Any non-default UA is accepted; confirmed on
	// stage 2026-06-24. (OBS's own player sends an Lavf/ffmpeg UA, which is why
	// playback itself was never affected.)
	radioProbeUserAgent = "tripbot-audio-watchdog"
)

// Deps are the OBS + radio hooks the watchdog calls. Injectable so the loop
// unit-tests without a real OBS WebSocket, meter, or network. DefaultDeps
// wires the production implementations.
type Deps struct {
	// MediaState returns the OBS media state of the background-audio source.
	MediaState func(context.Context) (string, error)
	// Level returns the latest peak output level (dBFS) and whether it is
	// fresh enough to trust.
	Level func() (db float64, fresh bool)
	// RadioReachable reports whether the radio stream served bytes just now.
	RadioReachable func(context.Context) bool
	// SwapToFallback points the source at a local bed for the outage: the album
	// when the share has tracks, the car-hum drone when it doesn't.
	SwapToFallback func(context.Context) error
	// SwapToRadio points the source back at its radio network stream.
	SwapToRadio func(context.Context) error
	// ActiveBed reports which bed the stream is meant to be playing. Only the
	// radio bed needs the outage machinery below: the car hum loops and has
	// nothing to ride out.
	ActiveBed func() beds.Bed
	// SourceIsLocal reports whether the background-audio source is pointed at a
	// local file right now. Read from OBS every tick rather than remembered
	// from the last swap: the console and chat repoint the same source, so
	// remembered state goes stale the moment an operator switches a bed.
	SourceIsLocal func(context.Context) (bool, error)
	// AdvanceAlbum queues the next fallback album track. Called when OBS
	// reports the media ended while the fallback is on air.
	AdvanceAlbum func(context.Context) error
	// Resync re-reads the live bed off OBS. Called on the first tick OBS
	// answers after it didn't: a restarted OBS boots onto its own default bed,
	// and a Store still holding the pre-restart bed reports the wrong gauge and
	// — worse — declines to advance an album it doesn't know is playing, so the
	// boot track ends and the stream goes silent (prod youtube, 2026-08-08).
	Resync func(context.Context)
}

// Config holds the watchdog's timing + threshold knobs.
type Config struct {
	Interval         time.Duration // how often to evaluate
	FailThreshold    int           // consecutive down ticks before swapping to fallback
	RecoverThreshold int           // consecutive reachable ticks before swapping back
	SilenceDB        float64       // peak level at/below which fresh audio counts as silent
	Cooldown         time.Duration // minimum time between swaps (anti-flap)
}

// maxProbeSkip caps the recovery probe's backoff at 31 sat-out ticks — a probe
// every ~3.7 minutes at the default interval, reached after six failures. The
// ceiling is a recovery latency: a returning edge waits up to that long to be
// noticed, which against outages measured in hours (and a fallback that is
// audible music) is worth trading for two orders of magnitude fewer requests.
// A day stuck on the fallback is ~12k connections at every tick and ~400 here.
//
// It is also self-defense. A station's server has firewalled us before, and the
// state that gets us blocked is the state that probes hardest: blocked means
// stranded on the fallback, and stranded means probing forever.
const maxProbeSkip = 31

// DefaultConfig is tuned for the failure we saw: ~20s of confirmed-down audio
// before falling back (3 × 7s ≈ a clear signal, not a momentary reconnect),
// ~30s of radio-healthy before swapping back, and a 2-minute cooldown so a
// flapping edge can't whipsaw the bed.
func DefaultConfig() Config {
	return Config{
		Interval:         7 * time.Second,
		FailThreshold:    3,
		RecoverThreshold: 4,
		SilenceDB:        -50,
		Cooldown:         2 * time.Minute,
	}
}

// DefaultDeps wires Watch's hooks to the real OBS WebSocket helpers, the live
// volume meter, and an HTTP radio probe.
func DefaultDeps(meter *VolumeMeter, store *beds.Store) Deps {
	return Deps{
		ActiveBed: func() beds.Bed {
			bed, _ := store.Current()
			return bed
		},
		AdvanceAlbum: store.Advance,
		Resync:       store.Detect,
		// Both OBS reads go over the meter's connection, which is already open
		// and subscribed: asked every tick, a fresh dial per call would spend
		// tens of thousands of connect + auth handshakes a day on two answers
		// the meter's socket can carry for free.
		MediaState: func(ctx context.Context) (string, error) {
			return meter.MediaInputState(ctx, BackgroundAudioInputName)
		},
		SourceIsLocal: func(ctx context.Context) (bool, error) {
			settings, err := meter.InputSettings(ctx, BackgroundAudioInputName)
			if err != nil {
				return false, err
			}
			local, _ := settings["is_local_file"].(bool)
			return local, nil
		},
		Level: meter.Level,
		// Both hooks read the station at call time rather than closing over it:
		// the console or chat can retune mid-outage, and swapping back to the
		// station we left would undo that.
		RadioReachable: func(ctx context.Context) bool {
			// No station to tune is the designed degraded state — the stream stays
			// on the fallback — not a probe failure worth a warning each attempt.
			url := beds.StreamURL(store.Station())
			if url == "" {
				return false
			}
			return defaultRadioReachable(ctx, url)
		},
		// Both swaps go through the Store: picking the fallback bed means
		// scanning the share and building a play order, and the Store is what
		// owns both. Neither touches the selected bed, so the outage machinery
		// keeps running and the stream comes back to the radio.
		SwapToFallback: store.SwapToFallback,
		SwapToRadio:    store.SwapToRadio,
	}
}

// radioProbeClient is dedicated to the reachability probe. DisableKeepAlives
// forces a fresh connection every probe: a periodic liveness check must not
// reuse a pooled connection, since a half-dead pooled conn to the radio would make
// a perfectly healthy edge look unreachable and strand us on the fallback bed.
// The explicit Timeout bounds the whole request as a backstop to the per-call
// context deadline.
var radioProbeClient = &http.Client{
	Timeout:   6 * time.Second,
	Transport: &http.Transport{DisableKeepAlives: true},
}

// defaultRadioReachable opens the radio stream and reads a byte: success means
// the edge is serving stream data. During an outage the connect hangs and the
// timeout trips, so this returns false. Distinct from "the website is up" —
// a station's site can 200 while its stream servers are dead.
//
// url is the live station's stream — the exact URL OBS is playing, so a station
// that is down on its own is caught, not masked by a healthy sibling channel.
//
// Every failure path logs why (at warn). The probe is only consulted while on
// the fallback bed, so in steady state it's silent; the logging is what makes a
// stuck-on-fallback state diagnosable instead of an unexplained silence (the
// original swallowed every error — see the 2026-06-24 stage test).
func defaultRadioReachable(ctx context.Context, url string) bool {
	cctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, url, nil)
	if err != nil {
		slog.WarnContext(ctx, "radio probe: build request failed", "err", err)
		return false
	}
	req.Header.Set("User-Agent", radioProbeUserAgent) // some Icecast servers reject Go's default UA
	resp, err := radioProbeClient.Do(req)
	if err != nil {
		slog.WarnContext(ctx, "radio probe: request failed", "err", err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		slog.WarnContext(ctx, "radio probe: unexpected status", "status", resp.StatusCode)
		return false
	}
	// Read a single byte — a live stream yields one immediately; a stalled
	// edge that completed the handshake but sends nothing trips the timeout.
	buf := make([]byte, 1)
	if _, err := io.ReadFull(resp.Body, buf); err != nil {
		slog.WarnContext(ctx, "radio probe: no stream bytes", "err", err)
		return false
	}
	return true
}

// Watch evaluates the background-audio source every cfg.Interval and keeps
// audible music on the stream: it swaps to the local bed after cfg.FailThreshold
// consecutive down ticks and swaps back after cfg.RecoverThreshold consecutive
// ticks of the radio being reachable, with cfg.Cooldown between any two swaps.
// Records the playing / level / on-fallback gauges every tick; radio_reachable
// only on the ticks that probe — which back off as an outage wears on, so it
// reads NoData rather than a stale value while nothing is waiting on the edge. Runs until ctx is canceled.
func Watch(ctx context.Context, platform string, deps Deps, cfg Config) {
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	slog.InfoContext(ctx, "obs background-audio watchdog started",
		"platform", platform,
		"interval", cfg.Interval, "fail_threshold", cfg.FailThreshold,
		"recover_threshold", cfg.RecoverThreshold, "cooldown", cfg.Cooldown)

	var (
		failMisses  int
		recoverHits int
		lastSwap    time.Time
		synced      bool // the Store has read the bed off this OBS process
		// probeSkip is how many ticks the recovery probe sits out between
		// attempts, and skipped how many of them have passed. See maxProbeSkip.
		probeSkip int
		skipped   int
	)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			db, fresh := deps.Level()
			if fresh {
				// Only record a level we actually trust. A stale meter (the
				// OBS WebSocket subscription dropped) would otherwise sit at
				// the -60 floor and read as false silence; recording nothing
				// instead lets the series go stale → NoData rather than
				// fake-silent, so dashboards show a gap and alerts don't fire.
				instrumentation.OBSBackgroundAudio.SetLevelDB(platform, db)
			}

			state, err := deps.MediaState(ctx)
			if err != nil {
				// OBS unreachable — can't read state or act. Let the meter
				// gauge / OBS-streaming alert carry it; don't advance counters
				// on a blind tick.
				slog.WarnContext(ctx, "audio watchdog: obs media state unavailable", "err", err)
				synced = false
				continue
			}
			if !synced {
				deps.Resync(ctx)
				instrumentation.OBSBackgroundAudio.IncResync(platform)
				synced = true
			}
			playing := state == obs.MediaStatePlaying
			instrumentation.OBSBackgroundAudio.SetPlaying(platform, playing)

			// The radio outage machinery below only makes sense while the radio is
			// the selected bed. On a local bed there's nothing to fall back
			// *to* and nothing to recover, so hold the counters at zero rather
			// than letting a quiet passage accumulate misses toward a swap that
			// would stomp the operator's choice.
			if deps.ActiveBed() != beds.Radio {
				failMisses, recoverHits = 0, 0
				// "On fallback" is a statement about the radio bed: a local bed
				// the operator chose is not a fallback, however local it is.
				instrumentation.OBSBackgroundAudio.SetOnFallback(platform, false)
				continue
			}

			// On the radio bed, a local file on the source means an earlier
			// fallback is still in place. Reading it off OBS rather than
			// remembering the last swap is what makes a bed switch made behind
			// the watchdog's back — the console reselecting the radio mid-outage —
			// visible: the source goes back to the dead stream, so the next tick
			// sees silence again instead of waiting out a recovery that already
			// happened.
			onFallback, err := deps.SourceIsLocal(ctx)
			if err != nil {
				slog.WarnContext(ctx, "audio watchdog: obs source settings unavailable", "err", err)
				continue
			}
			instrumentation.OBSBackgroundAudio.SetOnFallback(platform, onFallback)

			cooling := time.Since(lastSwap) < cfg.Cooldown

			if !onFallback {
				// Back on the real source, so the next outage starts probing at
				// full rate rather than inheriting the last one's backoff.
				probeSkip, skipped = 0, 0

				// On the real radio source: watch for the audio going down.
				silent := fresh && db <= cfg.SilenceDB
				if !obs.MediaStateDown(state) && !silent {
					failMisses = 0
					continue
				}
				failMisses++
				slog.WarnContext(ctx, "audio watchdog: background audio down",
					"state", state, "level_db", db, "level_fresh", fresh,
					"misses", failMisses, "threshold", cfg.FailThreshold)
				if failMisses < cfg.FailThreshold {
					continue
				}
				if cooling {
					slog.WarnContext(ctx, "audio watchdog: fallback suppressed by cooldown",
						"since_last_swap", time.Since(lastSwap), "cooldown", cfg.Cooldown)
					continue
				}
				slog.ErrorContext(ctx, "audio watchdog: radio down, swapping to local bed",
					"state", state)
				if err := deps.SwapToFallback(ctx); err != nil {
					slog.ErrorContext(ctx, "audio watchdog: swap to fallback failed", "err", err)
					continue
				}
				lastSwap = time.Now()
				failMisses = 0
				recoverHits = 0
				instrumentation.OBSBackgroundAudio.SetOnFallback(platform, true)
				instrumentation.OBSBackgroundAudio.IncSwap(platform, "to_fallback")
				continue
			}

			// The fallback album plays one track at a time, unlooped, so OBS ends
			// the media between tracks and each ending is the cue to queue the
			// next. The meter's playback-ended subscription normally gets there
			// first, in milliseconds; this is the backstop for an ending that
			// landed while the subscription was down, and the Store drops
			// whichever arrives second. It is also the one path that rescues a
			// fallback album with no play order onto the drone: down media there
			// never clears on its own. A no-op on the drone, which loops.
			if obs.MediaStateDown(state) {
				if err := deps.AdvanceAlbum(ctx); err != nil {
					slog.ErrorContext(ctx, "audio watchdog: fallback album advance failed", "err", err)
				}
			}

			// On the local fallback bed: wait for the radio to come back. This is the
			// only state whose outcome depends on the edge being up, so it's the
			// only state that probes: a connection opened on a tick that could
			// not act on the answer is load on the radio stream for nothing, and
			// enough of them across the fleet reads as abuse from one IP.
			//
			// Even here it backs off. An outage lasts hours and the fallback is
			// music, not silence, so there is nothing to gain from asking every
			// 7s — and everything to lose, since a fallback that never ends
			// means a probe that never stops.
			if skipped < probeSkip {
				skipped++
				continue
			}
			skipped = 0
			reachable := deps.RadioReachable(ctx)
			instrumentation.OBSBackgroundAudio.SetRadioReachable(platform, reachable)
			if !reachable {
				recoverHits = 0
				probeSkip = min(2*probeSkip+1, maxProbeSkip)
				continue
			}
			// One answer is enough to go back to probing every tick: the
			// remaining RecoverThreshold-1 confirmations are what the swap waits
			// on, and making those wait out a backoff would hold the stream on
			// the fallback for minutes after the edge came back.
			probeSkip = 0
			recoverHits++
			slog.InfoContext(ctx, "audio watchdog: radio reachable again",
				"hits", recoverHits, "threshold", cfg.RecoverThreshold)
			if recoverHits < cfg.RecoverThreshold {
				continue
			}
			if cooling {
				slog.InfoContext(ctx, "audio watchdog: recovery suppressed by cooldown",
					"since_last_swap", time.Since(lastSwap), "cooldown", cfg.Cooldown)
				continue
			}
			slog.InfoContext(ctx, "audio watchdog: radio recovered, swapping back")
			if err := deps.SwapToRadio(ctx); err != nil {
				slog.ErrorContext(ctx, "audio watchdog: swap back to radio failed", "err", err)
				continue
			}
			lastSwap = time.Now()
			recoverHits = 0
			instrumentation.OBSBackgroundAudio.SetOnFallback(platform, false)
			instrumentation.OBSBackgroundAudio.IncSwap(platform, "to_radio")
		}
	}
}
