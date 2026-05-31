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

# Listen on all interfaces, port 5900 — pulled from the rendered
# wayvnc.cfg in $XDG_RUNTIME_DIR. entrypoint.sh envsubsts the template
# from /opt/obs/config/wayvnc.cfg.tmpl with VNC_USERNAME + VNC_PASSWD
# before supervisord starts this program. Cert + key are also generated
# by entrypoint.sh at the paths the cfg points at.
#
# enable_auth=true in the cfg unlocks the encrypted security types
# (VeNCrypt for TigerVNC/RealVNC, Apple Diffie-Hellman for macOS Screen
# Sharing.app via relax_encryption=true) that wayvnc's default config
# leaves off — without enable_auth the cert paths are read-but-ignored
# and wayvnc only offers RFB "None" (security type 1), which Screen
# Sharing.app refuses with "Unable to communicate with localhost".
exec wayvnc --config="$XDG_RUNTIME_DIR/wayvnc.cfg"
