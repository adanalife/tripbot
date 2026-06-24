// Package audiowatchdog keeps audible background music on the Twitch stream
// when SomaFM drops. The Twitch music bed is the "Groove Salad Classic"
// ffmpeg_source (a SomaFM internet-radio stream); the 2026-06-23 outage showed
// it can go silent for minutes with no self-heal — OBS wedges on a clean EOF
// and stops retrying, a single edge feeds a stalling stream, or SomaFM's whole
// edge tier goes down. None of these recover without a manual nudge.
//
// The watchdog watches the source's media state + output level and, when audio
// is down past a debounce, swaps the source onto a local license-clean bed
// (the Car Hum FLAC already baked into the OBS image) so the stream isn't
// silent. It probes SomaFM in the background and swaps back once an edge is
// serving bytes again. All of it is the OBS-side analogue of the silent-
// disconnect stream watchdog in pkg/obs/watchdog — same injectable-deps shape
// so the decision loop unit-tests without a real OBS WebSocket. cmd/tripbot
// (Twitch only) is the sole consumer.
package audiowatchdog

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/adanalife/tripbot/pkg/obs"
)

// Cross-repo contracts with the adanalife/obs repo's scene config
// (config/Tripbot.json.tmpl) and Dockerfiles:
const (
	// BackgroundAudioInputName is the OBS source name of the Twitch music bed.
	// Must match the source "name" in the scene config.
	BackgroundAudioInputName = "Groove Salad Classic"

	// fallbackFile is the local, license-clean bed the source is pointed at
	// when SomaFM is unreachable. Baked into every OBS image by the carhum
	// build stage (COPY target in the obs repo's Dockerfile{,.arm64}); the
	// path is resolved by OBS, not tripbot. "idle" is the scene's default
	// Car Hum voicing on YouTube.
	fallbackFile = "/opt/tripbot/assets/carhum/car-hum-idle.flac"

	// somaFMProbeURL is the SomaFM endpoint the watchdog probes to decide when
	// it is safe to swap back. Matches the source's `input` URL in the scene
	// config — the round-robin ice.somafm.com hostname (4 edge IPs), so DNS
	// hands out a healthy edge rather than pinning one. A failed probe keeps us
	// on the safe local bed; per-edge probing is a tracked follow-up.
	somaFMProbeURL = "https://ice.somafm.com/gsclassic-128-mp3"
)

// Deps are the OBS + SomaFM hooks the watchdog calls. Injectable so the loop
// unit-tests without a real OBS WebSocket, meter, or network. DefaultDeps
// wires the production implementations.
type Deps struct {
	// MediaState returns the OBS media state of the background-audio source.
	MediaState func(context.Context) (string, error)
	// Level returns the latest peak output level (dBFS) and whether it is
	// fresh enough to trust.
	Level func() (db float64, fresh bool)
	// SomaFMReachable reports whether the SomaFM edge served bytes just now.
	SomaFMReachable func(context.Context) bool
	// SwapToFallback points the source at the local Car Hum bed.
	SwapToFallback func(context.Context) error
	// SwapToSomaFM points the source back at its SomaFM network stream.
	SwapToSomaFM func(context.Context) error
}

// Config holds the watchdog's timing + threshold knobs.
type Config struct {
	Interval         time.Duration // how often to evaluate
	FailThreshold    int           // consecutive down ticks before swapping to fallback
	RecoverThreshold int           // consecutive reachable ticks before swapping back
	SilenceDB        float64       // peak level at/below which fresh audio counts as silent
	Cooldown         time.Duration // minimum time between swaps (anti-flap)
}

// DefaultConfig is tuned for the failure we saw: ~20s of confirmed-down audio
// before falling back (3 × 7s ≈ a clear signal, not a momentary reconnect),
// ~30s of SomaFM-healthy before swapping back, and a 2-minute cooldown so a
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
// volume meter, and an HTTP SomaFM probe.
func DefaultDeps(meter *VolumeMeter) Deps {
	return Deps{
		MediaState: func(ctx context.Context) (string, error) {
			return obs.GetMediaInputState(ctx, BackgroundAudioInputName)
		},
		Level:           meter.Level,
		SomaFMReachable: defaultSomaFMReachable,
		SwapToFallback: func(ctx context.Context) error {
			return obs.SetInputLocalFileMode(ctx, BackgroundAudioInputName, fallbackFile)
		},
		SwapToSomaFM: func(ctx context.Context) error {
			return obs.SetInputNetworkMode(ctx, BackgroundAudioInputName)
		},
	}
}

// defaultSomaFMReachable opens the SomaFM edge and reads a byte: success means
// the edge is serving stream data. During an outage the connect hangs and the
// 5s client timeout trips, so this returns false. Distinct from "the website
// is up" — somafm.com can 200 while every ICEcast edge is dead.
func defaultSomaFMReachable(ctx context.Context) bool {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, somaFMProbeURL, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return false
	}
	// Read a single byte — a live stream yields one immediately; a stalled
	// edge that completed the handshake but sends nothing trips the timeout.
	buf := make([]byte, 1)
	n, err := io.ReadFull(resp.Body, buf)
	return n == 1 && (err == nil || err == io.EOF)
}

// Watch evaluates the background-audio source every cfg.Interval and keeps
// audible music on the stream: it swaps to the local bed after cfg.FailThreshold
// consecutive down ticks and swaps back after cfg.RecoverThreshold consecutive
// ticks of SomaFM being reachable, with cfg.Cooldown between any two swaps.
// Records the audio gauges every tick. Runs until ctx is cancelled.
func Watch(ctx context.Context, deps Deps, cfg Config) {
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	slog.InfoContext(ctx, "obs background-audio watchdog started",
		"interval", cfg.Interval, "fail_threshold", cfg.FailThreshold,
		"recover_threshold", cfg.RecoverThreshold, "cooldown", cfg.Cooldown)

	var (
		onFallback  bool
		failMisses  int
		recoverHits int
		lastSwap    time.Time
	)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reachable := deps.SomaFMReachable(ctx)
			instrumentation.OBSBackgroundAudio.SetSomaFMReachable(reachable)
			instrumentation.OBSBackgroundAudio.SetOnFallback(onFallback)

			db, fresh := deps.Level()
			if fresh {
				// Only record a level we actually trust. A stale meter (the
				// OBS WebSocket subscription dropped) would otherwise sit at
				// the -60 floor and read as false silence; recording nothing
				// instead lets the series go stale → NoData rather than
				// fake-silent, so dashboards show a gap and alerts don't fire.
				instrumentation.OBSBackgroundAudio.SetLevelDB(db)
			}

			state, err := deps.MediaState(ctx)
			if err != nil {
				// OBS unreachable — can't read state or act. Let the meter
				// gauge / OBS-streaming alert carry it; don't advance counters
				// on a blind tick.
				slog.WarnContext(ctx, "audio watchdog: obs media state unavailable", "err", err)
				continue
			}
			playing := state == obs.MediaStatePlaying
			instrumentation.OBSBackgroundAudio.SetPlaying(playing)

			cooling := time.Since(lastSwap) < cfg.Cooldown

			if !onFallback {
				// On the real SomaFM source: watch for the audio going down.
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
				slog.ErrorContext(ctx, "audio watchdog: SomaFM down, swapping to local bed",
					"state", state, "fallback_file", fallbackFile)
				if err := deps.SwapToFallback(ctx); err != nil {
					slog.ErrorContext(ctx, "audio watchdog: swap to fallback failed", "err", err)
					continue
				}
				onFallback = true
				lastSwap = time.Now()
				failMisses = 0
				recoverHits = 0
				instrumentation.OBSBackgroundAudio.SetOnFallback(true)
				instrumentation.OBSBackgroundAudio.IncSwap("to_fallback")
				continue
			}

			// On the local fallback bed: wait for SomaFM to come back.
			if !reachable {
				recoverHits = 0
				continue
			}
			recoverHits++
			slog.InfoContext(ctx, "audio watchdog: SomaFM reachable again",
				"hits", recoverHits, "threshold", cfg.RecoverThreshold)
			if recoverHits < cfg.RecoverThreshold {
				continue
			}
			if cooling {
				slog.InfoContext(ctx, "audio watchdog: recovery suppressed by cooldown",
					"since_last_swap", time.Since(lastSwap), "cooldown", cfg.Cooldown)
				continue
			}
			slog.InfoContext(ctx, "audio watchdog: SomaFM recovered, swapping back")
			if err := deps.SwapToSomaFM(ctx); err != nil {
				slog.ErrorContext(ctx, "audio watchdog: swap back to SomaFM failed", "err", err)
				continue
			}
			onFallback = false
			lastSwap = time.Now()
			recoverHits = 0
			instrumentation.OBSBackgroundAudio.SetOnFallback(false)
			instrumentation.OBSBackgroundAudio.IncSwap("to_somafm")
		}
	}
}
