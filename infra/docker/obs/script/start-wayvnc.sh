#!/usr/bin/env bash
# Supervisor program: serve the headless sway output over VNC on :5900.
#
# Replaces the entrypoint's previous `x11vnc -display :0 ...` line.
# Waits for sway to publish its Wayland socket before launching — wayvnc
# connects as a Wayland client and can't bootstrap before the compositor.
set -euo pipefail

export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/tmp/runtime-root}"
export WAYLAND_DISPLAY="${WAYLAND_DISPLAY:-wayland-1}"

# Block until the sway socket exists. supervisor's startretries=10 catches
# the case where sway is slow to come up after a restart.
for _ in $(seq 1 60); do
  if [[ -S "$XDG_RUNTIME_DIR/$WAYLAND_DISPLAY" ]]; then
    break
  fi
  sleep 0.5
done

if [[ ! -S "$XDG_RUNTIME_DIR/$WAYLAND_DISPLAY" ]]; then
  echo "wayvnc: Wayland socket $XDG_RUNTIME_DIR/$WAYLAND_DISPLAY never appeared" >&2
  exit 1
fi

# Listen on all interfaces, port 5900 (matches the EXPOSE in the
# Dockerfile and the Service definition in k8s/apps/obs/base/). Auth
# stays off — the Service is cluster-internal and reached via
# `task obs:vnc:up` port-forward, mirroring the previous x11vnc setup.
exec wayvnc 0.0.0.0 5900
