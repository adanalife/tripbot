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
"""

import argparse
import numpy as np
from scipy import signal


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


def build_channel(n, sr, base_hz, rng):
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

    mix = 0.55 * sub + 1.0 * road + 0.12 * air + 0.16 * eng
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
    p.add_argument("--base-hz", type=float, default=52.0, help="engine fundamental")
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

    ss = np.random.SeedSequence(args.seed)
    n = int(args.duration * args.sr)

    # Independent noise per channel -> natural stereo width, no harsh phasing.
    rngs = [np.random.default_rng(s) for s in ss.spawn(2)]
    left = build_channel(n, args.sr, args.base_hz, rngs[0])
    right = build_channel(n, args.sr, args.base_hz, rngs[1])
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
    print(f"wrote {args.out}  ({args.duration:.0f}s, {args.sr} Hz, stereo)")


if __name__ == "__main__":
    main()
