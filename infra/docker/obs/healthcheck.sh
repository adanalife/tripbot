#!/usr/bin/env bash
# Called by infra/docker/obs/Dockerfile{,.arm64} (HEALTHCHECK CMD).
# Healthy when:
#   - the obs process is alive
#   - the X server responds
#   - the OBS-32 safe-mode crash dialog is NOT up
#
# Why specifically the safe-mode dialog: it appears on top of an empty
# desktop instead of the OBS main window, blocking everything until a
# human dismisses it. Other OBS modals ("Missing Files", etc.) leave
# the main window functional and are tolerable. Detection: walk
# _NET_CLIENT_LIST and fail when any window has WM_CLASS=obs,
# _NET_WM_STATE_MODAL, and a WM_NAME containing "Crash Detected".
set -u
export DISPLAY=:0

pgrep -x obs >/dev/null || exit 1
xdpyinfo >/dev/null 2>&1 || exit 1

for wid in $(xprop -root _NET_CLIENT_LIST 2>/dev/null | grep -oE '0x[0-9a-fA-F]+'); do
  cls=$(xprop -id "$wid" WM_CLASS 2>/dev/null)
  state=$(xprop -id "$wid" _NET_WM_STATE 2>/dev/null)
  name=$(xprop -id "$wid" WM_NAME 2>/dev/null)
  if [[ "$cls"   == *'"obs"'* ]] && \
     [[ "$state" == *_NET_WM_STATE_MODAL* ]] && \
     [[ "$name"  == *"Crash Detected"* ]]; then
    echo "OBS safe-mode dialog blocking the main window: $wid" >&2
    exit 1
  fi
done

exit 0
