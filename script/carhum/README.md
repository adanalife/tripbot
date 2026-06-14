# carhum — synthesized car-interior drone

`carhum.py` generates a calming car-interior ambience (road roar + faint engine
hum + cabin air) entirely from filtered noise and a few low sine harmonics. It's
100% synthesized, so it carries **no licensing risk** — unlike a ripped
recording, it can't earn a YouTube Content ID claim or copyright strike.

It's the source of [`assets/audio/car-hum-loop.flac`](../../assets/audio/car-hum-loop.flac),
the background-audio bed the OBS **YouTube** scene plays in place of the
SomaFM source (which is stripped on YouTube — see
`infra/docker/obs/entrypoint.sh`).

## Regenerating the asset

The committed loop was rendered with:

```sh
uv run carhum.py --duration 240 --base-hz 46 --seed 5 --loop 6 --out car-hum-loop.wav
ffmpeg -i car-hum-loop.wav -compression_level 8 ../../assets/audio/car-hum-loop.flac
```

- `--seed 5 --base-hz 46` is the "v3-low-engine" character (low, mellow drone).
  Change the seed to re-roll the noise/breathing entirely; lower `--base-hz`
  (40–48) for a bigger/calmer engine, higher for a more compact one.
- `--loop 6` crossfades the tail back over the head so the file loops with **no
  audible seam** when OBS repeats it.
- FLAC keeps the loop gapless (no encoder padding) and compresses this
  low-frequency signal to a few MB.

`uv` reads the inline PEP 723 dependency block at the top of the script, so no
separate venv setup is needed.

## Why it doesn't sound digital

Each layer breathes on its own slow LFO, the engine fundamental wanders on a
smoothed random walk, and the two stereo channels use independent noise for
natural width — so nothing reads as a static, looping synth tone.
