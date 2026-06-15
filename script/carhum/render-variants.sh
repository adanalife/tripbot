#!/usr/bin/env bash
# Render the car-sound variant set to seamless-looping FLACs.
#
# Runs at Docker BUILD time (the obs image's `carhum` builder stage), never at
# runtime — generating in a throwaway stage keeps numpy/scipy out of the final
# image while still shipping procedurally-generated (not git-committed) audio.
# Needs python3 with numpy+scipy importable and ffmpeg on PATH.
#
# The variant NAMES + COUNT here are a contract shared three ways:
#   - the `carhum` builder stage + COPY in infra/docker/obs/Dockerfile{,.arm64}
#   - the carSounds registry in pkg/chatbot/carsound.go (the !carsound command)
# Keep all three in sync when adding/removing a variant.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
out="${1:?usage: render-variants.sh <out-dir>}"
mkdir -p "$out"

DURATION=240   # 4-minute bed
LOOP=6         # crossfade seconds -> seam-free loop (verified by splice test)

# "<preset-name>:<seed>" — the preset name doubles as the variant name; seeds
# are fixed so the image build is reproducible.
variants="idle:5 highway:2 backroad:7 mountain:9"

for v in $variants; do
  name="${v%%:*}"
  seed="${v##*:}"
  wav="$out/$name.wav"
  flac="$out/car-hum-$name.flac"
  echo ">> rendering $name (seed $seed)"
  python3 "$here/carhum.py" --preset "$name" --duration "$DURATION" \
    --loop "$LOOP" --seed "$seed" --out "$wav"
  # FLAC is gapless (no encoder padding), so the seam-free loop survives encode.
  ffmpeg -nostdin -loglevel error -y -i "$wav" -compression_level 8 "$flac"
  rm -f "$wav"
  echo "   -> $flac"
done

echo "done: $(find "$out" -name '*.flac' | wc -l | tr -d ' ') variants in $out"
