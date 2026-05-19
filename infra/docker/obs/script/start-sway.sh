#!/usr/bin/env bash
# Supervisor program: launch the sway compositor with headless backends.
#
# Replaces the entrypoint's previous `Xvfb $DISPLAY -screen 0 ... &` line.
# sway uses GBM under the hood to render directly to the iGPU (/dev/dri/card0),
# unlike Xvfb which is software-only.
set -euo pipefail

# Wayland sockets / runtime files live here. tmpfs at /tmp keeps the
# socket out of any persistent volume.
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/tmp/runtime-root}"
mkdir -p "$XDG_RUNTIME_DIR"
chmod 0700 "$XDG_RUNTIME_DIR"

# wlroots: create a virtual headless output instead of probing for real
# DRM connectors (host has none attached). Belt-and-braces: also tell it
# not to wait for input devices that don't exist in a container.
export WLR_BACKENDS=headless
export WLR_LIBINPUT_NO_DEVICES=1

# Pin the rendered output to the iGPU via /dev/dri/card0 (primary node,
# KMS-capable) when present — renderD128 alone isn't enough for sway,
# which uses GBM allocation that needs card0. Skip the pin when the
# host doesn't expose a DRM device at all (Mac k3d / stage-1) so sway
# can fall back to a software path; OBS rendering will be slow there
# but the pod still boots for end-to-end pipeline testing.
if [[ -e /dev/dri/card0 ]]; then
  export WLR_RENDER_DRM_DEVICE=/dev/dri/card0
else
  echo "sway: no /dev/dri/card0; running with software renderer (Mac dev / stage-1)" >&2
  export WLR_RENDERER=pixman
fi

export SWAYSOCK="$XDG_RUNTIME_DIR/sway-ipc.sock"

exec sway -c /opt/obs/config/sway-headless.conf
