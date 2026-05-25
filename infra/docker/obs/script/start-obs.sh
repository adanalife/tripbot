#!/usr/bin/env bash
# Supervisor program: launch OBS as a Wayland-native Qt client.
#
# Replaces the entrypoint's previous `exec obs ...` line. Waits for sway's
# Wayland socket before starting — OBS's Qt6 platform plugin can't connect
# to the compositor before it's up.
set -euo pipefail

export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/tmp/runtime-root}"
export WAYLAND_DISPLAY="${WAYLAND_DISPLAY:-wayland-1}"

# Force Qt to load its Wayland QPA plugin (qt6-wayland package). Without
# this Qt picks the highest-priority available platform, and on a system
# with both X11 and Wayland libs present it sometimes guesses wrong.
export QT_QPA_PLATFORM=wayland

# Block until the sway socket exists.
for _ in $(seq 1 60); do
  if [[ -S "$XDG_RUNTIME_DIR/$WAYLAND_DISPLAY" ]]; then
    break
  fi
  sleep 0.5
done

if [[ ! -S "$XDG_RUNTIME_DIR/$WAYLAND_DISPLAY" ]]; then
  echo "obs: Wayland socket $XDG_RUNTIME_DIR/$WAYLAND_DISPLAY never appeared" >&2
  exit 1
fi

# OBS args mirror the previous entrypoint invocation. --startstreaming
# only makes sense when service.json was rendered (STREAM_KEY was set);
# entrypoint.sh writes a sentinel into the obs profile dir to signal that.
obs_args=(--disable-shutdown-check --collection 'Tripbot' --profile 'ADanaLife' --scene 'Main')

OBS_HOME="${HOME:-/root}/.config/obs-studio"
if [[ -f "$OBS_HOME/basic/profiles/ADanaLife/service.json" ]]; then
  obs_args+=(--startstreaming)
fi

exec obs "${obs_args[@]}"
