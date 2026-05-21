#!/usr/bin/env bash
# Supervisor program: launch the sway compositor with headless backends.
#
# Replaces the entrypoint's previous `Xvfb $DISPLAY -screen 0 ... &` line.
# sway uses GBM under the hood to render directly to the iGPU's render
# node (/dev/dri/renderD128), unlike Xvfb which is software-only.
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

# Pin the rendered output to the iGPU via the render node (renderD128).
# wlroots' renderer specifically REQUIRES a DRM render node and rejects
# the primary node /dev/dri/card0 with `'/dev/dri/card0' is not a DRM
# render node` — card0 is for KMS/scanout, not for GPU compute, which
# is all the headless backend needs. Skip the pin when the host doesn't
# expose a render node at all (Mac k3d / stage-1) so sway falls back to
# a software renderer; OBS rendering is slow there but the pod still
# boots for end-to-end pipeline testing.
if [[ -e /dev/dri/renderD128 ]]; then
  export WLR_RENDER_DRM_DEVICE=/dev/dri/renderD128
else
  echo "sway: no /dev/dri/renderD128; running with software renderer (Mac dev / stage-1)" >&2
  export WLR_RENDERER=pixman
fi

export SWAYSOCK="$XDG_RUNTIME_DIR/sway-ipc.sock"

exec sway -c /opt/obs/config/sway-headless.conf
