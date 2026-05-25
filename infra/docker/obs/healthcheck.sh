#!/usr/bin/env bash
# Called by infra/docker/obs/Dockerfile{,.arm64} (HEALTHCHECK CMD) and by
# the OBS deployment's livenessProbe. Healthy when:
#   - the obs process is alive
#   - sway is reachable (compositor is up)
#   - the OBS-32 safe-mode crash dialog is NOT up
#
# Why specifically the safe-mode dialog: it appears on top of an empty
# desktop instead of the OBS main window, blocking everything until a
# human dismisses it. Other OBS modals ("Missing Files", etc.) leave
# the main window functional and are tolerable. Detection: walk the
# sway tree and fail when any node has a "Crash Detected" title.
set -u
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/tmp/runtime-root}"
export WAYLAND_DISPLAY="${WAYLAND_DISPLAY:-wayland-1}"
export SWAYSOCK="$XDG_RUNTIME_DIR/sway-ipc.sock"

pgrep -x obs >/dev/null || exit 1

# `swaymsg -t get_tree` proves the compositor is responsive. We don't
# need jq to walk the tree — a grep on the title field catches the
# safe-mode dialog without adding another package dep.
tree=$(swaymsg -t get_tree 2>/dev/null) || exit 1

if grep -qE '"name":\s*"[^"]*Crash Detected[^"]*"' <<<"$tree"; then
  echo "OBS safe-mode dialog blocking the main window" >&2
  exit 1
fi

exit 0
