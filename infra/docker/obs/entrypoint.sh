#!/usr/bin/env bash
set -euo pipefail

OBS_HOME="${HOME:-/root}/.config/obs-studio"
mkdir -p "$OBS_HOME/basic/profiles/Untitled" "$OBS_HOME/basic/scenes"

# OBS 32 split the legacy global.ini in two: app-level settings stayed in
# global.ini (BrowserHWAccel etc.) and user-preference settings moved to
# user.ini. Seed both so OBS sees a complete config and never prompts about
# migration.
cp /opt/obs/config/global.ini "$OBS_HOME/global.ini"
cp /opt/obs/config/user.ini   "$OBS_HOME/user.ini"
cp /opt/obs/config/basic.ini  "$OBS_HOME/basic/profiles/Untitled/basic.ini"
envsubst < /opt/obs/config/Tripbot.json.tmpl > "$OBS_HOME/basic/scenes/Tripbot.json"

obs_args=(--disable-shutdown-check --collection 'Tripbot' --profile 'Untitled' --scene 'Main')

if [[ -n "${STREAM_KEY:-}" ]]; then
  echo "STREAM_KEY set; configuring Twitch and starting stream."
  envsubst < /opt/obs/config/service.json.tmpl \
    > "$OBS_HOME/basic/profiles/Untitled/service.json"
  obs_args+=(--startstreaming)
else
  echo "STREAM_KEY not set; OBS will start idle. VNC into :5902 to inspect."
fi

export DISPLAY=:0

rm -f /tmp/.X11-unix/X0 /tmp/.X0-lock
Xvfb "$DISPLAY" -screen 0 1920x1200x24 &
sleep 1

# Pin the OBS main window to the center of the 1920x1200 Xvfb display via
# fluxbox's apps file. Without this rule OBS opens at fluxbox's default
# top-left corner, leaving most of the framebuffer empty in VNC. Match by
# WM_CLASS=obs (stable across version/profile/scene title changes).
mkdir -p "$HOME/.fluxbox"
cat > "$HOME/.fluxbox/apps" <<'EOF'
[app] (class=obs)
  [Position] (CENTER)	{0 0}
[end]
EOF

fluxbox &
sleep 1

x11vnc -display "$DISPLAY" -forever -shared -rfbport 5900 -passwd "${VNC_PASSWD:-123456}" &

exec obs "${obs_args[@]}"
