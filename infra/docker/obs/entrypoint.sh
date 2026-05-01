#!/usr/bin/env bash
set -euo pipefail

OBS_HOME="${HOME:-/root}/.config/obs-studio"
mkdir -p "$OBS_HOME/basic/profiles/Untitled" "$OBS_HOME/basic/scenes"

cp /opt/obs/config/global.ini "$OBS_HOME/global.ini"
cp /opt/obs/config/basic.ini  "$OBS_HOME/basic/profiles/Untitled/basic.ini"
cp /opt/obs/config/Tripbot.json "$OBS_HOME/basic/scenes/Tripbot.json"

obs_args=(--disable-shutdown-check --collection 'Tripbot' --profile 'Untitled' --scene 'Test')

if [[ -n "${STREAM_KEY:-}" ]]; then
  echo "STREAM_KEY set; configuring Twitch and starting stream."
  envsubst < /opt/obs/config/service.json.tmpl \
    > "$OBS_HOME/basic/profiles/Untitled/service.json"
  obs_args+=(--startstreaming)
else
  echo "STREAM_KEY not set; OBS will start idle. VNC into :5902 to inspect."
fi

export DISPLAY=:0
Xvfb :0 -screen 0 1920x1080x24 -nolisten tcp &
for i in {1..30}; do [[ -S /tmp/.X11-unix/X0 ]] && break; sleep 0.2; done

fluxbox >/dev/null 2>&1 &
x11vnc -display :0 -forever -shared -nopw -rfbport 5900 -quiet >/dev/null 2>&1 &

exec obs "${obs_args[@]}"
