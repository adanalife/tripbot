#!/usr/bin/env bash
set -euo pipefail

OBS_HOME="${HOME:-/root}/.config/obs-studio"
mkdir -p "$OBS_HOME/basic/profiles/Untitled" "$OBS_HOME/basic/scenes"

# OBS 32 renamed the global config from global.ini to user.ini and prompts
# about migration if it sees a pre-31 global.ini. We seed user.ini directly
# and explicitly delete any stale global.ini to keep the migration path
# unambiguous when an older container's config dir survives a rebuild.
rm -f "$OBS_HOME/global.ini"
cp /opt/obs/config/user.ini  "$OBS_HOME/user.ini"
cp /opt/obs/config/basic.ini "$OBS_HOME/basic/profiles/Untitled/basic.ini"
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

fluxbox &
sleep 1

x11vnc -display "$DISPLAY" -forever -shared -rfbport 5900 -passwd "${VNC_PASSWD:-123456}" &

exec obs "${obs_args[@]}"
