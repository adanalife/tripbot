#!/usr/bin/env bash
# Healthy when:
#   - the obs process is alive
#   - the X server responds
#   - no OBS modal dialog is wedging the UI
#
# The modal check exists because OBS surfaces blocking, unrecoverable prompts
# (OBS-32 safe-mode after a crashy restart, "Missing Files" when scene assets
# vanish, etc.) that no automation in the container can dismiss. Without this
# the container looks healthy while the app is actually inert. Detection: walk
# _NET_CLIENT_LIST and fail if any window has WM_CLASS=obs and
# _NET_WM_STATE_MODAL — that combination is unique to OBS's blocking dialogs;
# normal OBS dock panes (Stats, Output Timer, etc.) are WM_CLASS=obs but not
# modal.
set -u
export DISPLAY=:0

pgrep -x obs >/dev/null || exit 1
xdpyinfo >/dev/null 2>&1 || exit 1

for wid in $(xprop -root _NET_CLIENT_LIST 2>/dev/null | grep -oE '0x[0-9a-fA-F]+'); do
  cls=$(xprop -id "$wid" WM_CLASS 2>/dev/null)
  state=$(xprop -id "$wid" _NET_WM_STATE 2>/dev/null)
  if [[ "$cls" == *'"obs"'* ]] && [[ "$state" == *_NET_WM_STATE_MODAL* ]]; then
    name=$(xprop -id "$wid" WM_NAME 2>/dev/null | sed -n 's/^WM_NAME(STRING) = //p')
    echo "OBS modal dialog blocking: $name ($wid)" >&2
    exit 1
  fi
done

exit 0
