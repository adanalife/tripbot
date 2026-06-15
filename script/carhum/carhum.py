# /// script
# requires-python = ">=3.10"
# dependencies = ["numpy", "scipy"]
# ///
"""
carhum.py — synthesize a calming car-interior drone (road + engine + cabin air).

100% generated, so it's license-clean for a monetized YouTube stream.
Designed as a relaxing backing bed: gentle, broadband, slowly "breathing" so it
never sounds static or obviously digital. Render long, or make a seamless loop.

Examples:
    uv run carhum.py --duration 60 --out sample.wav
    uv run carhum.py --duration 1800 --out bed_30min.wav        # 30-minute bed
    uv run carhum.py --duration 120 --loop 6 --out loop.wav     # seamless 2-min loop
    uv run carhum.py --preset highway --loop 6 --out highway.wav  # a character preset
"""

import argparse
import numpy as np
from scipy import signal


# Character presets — each is a base engine pitch plus the mix weights of the
# four layers (sub rumble / road roar / cabin air / engine drone). They give
# four audibly distinct "car" beds to cycle between on stream. `--base-hz`
# still wins if passed explicitly. The un-presetted default ("classic") keeps
# the original voicing so existing standalone renders are unchanged.
DEFAULT_WEIGHTS = {"sub": 0.55, "road": 1.00, "air": 0.12, "eng": 0.16}
PRESETS = {
    # stopped/slow: low rumble, engine forward, little road roar.
    "idle": {"base_hz": 40, "sub": 0.55, "road": 0.70, "air": 0.10, "eng": 0.30},
    # fast tarmac: road roar dominant, bright cabin air, engine sunk.
    "highway": {"base_hz": 60, "sub": 0.60, "road": 1.15, "air": 0.18, "eng": 0.10},
    # balanced mid — closest to the original "low engine" voicing.
    "backroad": {"base_hz": 50, "sub": 0.55, "road": 1.00, "air": 0.14, "eng": 0.18},
    # airy/open: more cabin hiss up top, softer road, gentle engine.
    "mountain": {"base_hz": 46, "sub": 0.50, "road": 0.85, "air": 0.22, "eng": 0.14},
}


def colored_noise(n, alpha, rng):
    """Power-law noise via spectral shaping. alpha=1 pink, alpha=2 brown/red."""
    white = rng.standard_normal(n)
    spectrum = np.fft.rfft(white)
    freqs = np.fft.rfftfreq(n, d=1.0)
    scale = np.ones_like(freqs)
    scale[1:] = freqs[1:] ** (-alpha / 2.0)  # leave DC alone
    out = np.fft.irfft(spectrum * scale, n=n)
    return out / (np.std(out) + 1e-12)


def butter_apply(x, sr, cutoff, btype, order=4):
    sos = signal.butter(order, cutoff, btype=btype, fs=sr, output="sos")
    return signal.sosfiltfilt(sos, x)


def slow_lfo(t, sr, rate_hz, depth, rng):
    """A gentle breathing envelope in [1-depth, 1+depth], with a random phase."""
    phase = rng.uniform(0, 2 * np.pi)
    return 1.0 + depth * np.sin(2 * np.pi * rate_hz * t + phase)


def smooth_random_walk(n, sr, smoothing_hz, rng):
    """Slowly drifting signal in ~[-1,1] — used to wander the engine pitch."""
    walk = np.cumsum(rng.standard_normal(n))
    walk = butter_apply(walk, sr, smoothing_hz, "low", order=2)
    walk -= walk.mean()
    return walk / (np.max(np.abs(walk)) + 1e-12)


def engine_drone(t, sr, base_hz, n_harmonics, rng):
    """Low engine hum: a drifting fundamental plus decaying harmonics."""
    n = len(t)
    drift = smooth_random_walk(n, sr, 0.08, rng) * 2.5  # +/- 2.5 Hz wander
    inst_freq = base_hz + drift
    phase = 2 * np.pi * np.cumsum(inst_freq) / sr
    out = np.zeros(n)
    for h in range(1, n_harmonics + 1):
        amp = 1.0 / (h**1.6)  # higher harmonics fall off fast -> mellow
        # each harmonic breathes a little independently
        amp_env = slow_lfo(t, sr, rng.uniform(0.04, 0.12), 0.25, rng)
        out += amp * amp_env * np.sin(h * phase)
    return out / (np.std(out) + 1e-12)


def build_channel(n, sr, base_hz, rng, weights=DEFAULT_WEIGHTS):
    t = np.arange(n) / sr

    # 1. Sub rumble — chassis/road, felt more than heard. Very low.
    sub = colored_noise(n, alpha=2.0, rng=rng)
    sub = butter_apply(sub, sr, 65, "low")
    sub *= slow_lfo(t, sr, 0.05, 0.20, rng)

    # 2. Road roar — the dominant layer. Brownish, rolled off in the low-mids.
    road = colored_noise(n, alpha=1.7, rng=rng)
    road = butter_apply(road, sr, [40, 450], "band")
    road *= slow_lfo(t, sr, 0.07, 0.22, rng) * slow_lfo(t, sr, 0.11, 0.12, rng)

    # 3. Cabin air — faint airy hiss up top so it's not a dull lump.
    air = colored_noise(n, alpha=1.0, rng=rng)
    air = butter_apply(air, sr, [300, 1600], "band")
    air *= slow_lfo(t, sr, 0.09, 0.30, rng)

    # 4. Engine drone — subtle, keeps it reading as "car" not "wind tunnel".
    eng = engine_drone(t, sr, base_hz, n_harmonics=4, rng=rng)
    eng = butter_apply(eng, sr, 320, "low")
    eng *= slow_lfo(t, sr, 0.06, 0.20, rng)

    mix = (
        weights["sub"] * sub
        + weights["road"] * road
        + weights["air"] * air
        + weights["eng"] * eng
    )
    return mix


def seamless_loop(stereo, sr, xfade_s):
    """Crossfade the tail back over the head so the file loops with no seam."""
    xf = int(xfade_s * sr)
    if xf <= 0 or 2 * xf >= len(stereo):
        return stereo
    head, body, tail = stereo[:xf], stereo[xf:-xf], stereo[-xf:]
    fade = np.linspace(0, 1, xf)[:, None]
    blended = tail * (1 - fade) + head * fade
    return np.concatenate([blended, body], axis=0)


def main():
    p = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    p.add_argument("--duration", type=float, default=60.0, help="seconds")
    p.add_argument("--sr", type=int, default=48000, help="sample rate")
    p.add_argument(
        "--preset",
        choices=sorted(PRESETS),
        default=None,
        help="character preset (sets engine pitch + layer mix); see PRESETS",
    )
    p.add_argument(
        "--base-hz",
        type=float,
        default=None,
        help="engine fundamental (overrides the preset's; default 52)",
    )
    p.add_argument("--seed", type=int, default=None, help="reproducible render")
    p.add_argument(
        "--loop",
        type=float,
        default=0.0,
        help="crossfade seconds for a seamless loop (0 = off)",
    )
    p.add_argument(
        "--peak-dbfs", type=float, default=-3.0, help="normalize peak to this level"
    )
    p.add_argument("--out", default="carhum.wav")
    args = p.parse_args()

    # Resolve preset -> mix weights + engine pitch. An explicit --base-hz always
    # wins; otherwise fall back to the preset's pitch, then the classic default.
    preset = PRESETS.get(args.preset, {})
    weights = {k: preset.get(k, DEFAULT_WEIGHTS[k]) for k in DEFAULT_WEIGHTS}
    base_hz = args.base_hz if args.base_hz is not None else preset.get("base_hz", 52.0)

    ss = np.random.SeedSequence(args.seed)
    n = int(args.duration * args.sr)

    # Independent noise per channel -> natural stereo width, no harsh phasing.
    rngs = [np.random.default_rng(s) for s in ss.spawn(2)]
    left = build_channel(n, args.sr, base_hz, rngs[0], weights)
    right = build_channel(n, args.sr, base_hz, rngs[1], weights)
    stereo = np.stack([left, right], axis=1)

    if args.loop > 0:
        stereo = seamless_loop(stereo, args.sr, args.loop)
    else:
        # short fade in/out so a non-looped file doesn't click at the edges
        fl = min(int(1.5 * args.sr), len(stereo) // 4)
        env = np.ones(len(stereo))
        env[:fl] = np.linspace(0, 1, fl)
        env[-fl:] = np.linspace(1, 0, fl)
        stereo *= env[:, None]

    peak = np.max(np.abs(stereo)) + 1e-12
    target = 10 ** (args.peak_dbfs / 20.0)
    stereo = stereo / peak * target

    pcm = (np.clip(stereo, -1, 1) * 32767).astype(np.int16)
    from scipy.io import wavfile

    wavfile.write(args.out, args.sr, pcm)
    tag = args.preset or "classic"
    print(
        f"wrote {args.out}  ({args.duration:.0f}s, {args.sr} Hz, stereo, "
        f"preset={tag}, base={base_hz:.0f}Hz)"
    )


if __name__ == "__main__":
    main()
