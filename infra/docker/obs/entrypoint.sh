#!/usr/bin/env bash
set -euo pipefail

echo "OBS container, tripbot version $(cat /etc/tripbot/version 2>/dev/null || echo dev) (sha: $(cat /etc/tripbot/sha 2>/dev/null || echo unknown))"

OBS_HOME="${HOME:-/root}/.config/obs-studio"
mkdir -p "$OBS_HOME/basic/profiles/ADanaLife" "$OBS_HOME/basic/scenes"

# Expand quality preset into individual encoder params before envsubst.
# OBS_QUALITY_PRESET=low  → 720p30, 1500 kbps (staging / dev laptops)
# OBS_QUALITY_PRESET=high → 1080p60, 6000 kbps (production, Twitch max)
case "${OBS_QUALITY_PRESET:-high}" in
  low)
    export OBS_OUTPUT_WIDTH=1280
    export OBS_OUTPUT_HEIGHT=720
    export OBS_FPS_COMMON=30
    export OBS_VIDEO_BITRATE=1500
    export OBS_AUDIO_BITRATE=128
    export OBS_ENCODER_PRESET=ultrafast
    echo "OBS quality preset: low (720p30, 1500 kbps)"
    ;;
  *)
    export OBS_OUTPUT_WIDTH=1920
    export OBS_OUTPUT_HEIGHT=1080
    export OBS_FPS_COMMON=60
    export OBS_VIDEO_BITRATE=6000
    export OBS_AUDIO_BITRATE=160
    export OBS_ENCODER_PRESET=veryfast
    echo "OBS quality preset: high (1080p60, 6000 kbps)"
    ;;
esac

# OBS 32 split the legacy global.ini in two: app-level settings stayed in
# global.ini (BrowserHWAccel etc.) and user-preference settings moved to
# user.ini. Seed both so OBS sees a complete config and never prompts about
# migration.
cp /opt/obs/config/global.ini "$OBS_HOME/global.ini"
cp /opt/obs/config/user.ini   "$OBS_HOME/user.ini"
envsubst < /opt/obs/config/basic.ini.tmpl > "$OBS_HOME/basic/profiles/ADanaLife/basic.ini"
envsubst < /opt/obs/config/Tripbot.json.tmpl > "$OBS_HOME/basic/scenes/Tripbot.json"

mkdir -p "$OBS_HOME/plugin_config/obs-websocket"
envsubst < /opt/obs/config/obs-websocket.json.tmpl > "$OBS_HOME/plugin_config/obs-websocket/config.json"

obs_args=(--disable-shutdown-check --collection 'Tripbot' --profile 'ADanaLife' --scene 'Main')

if [[ -n "${STREAM_KEY:-}" ]]; then
  echo "STREAM_KEY set; configuring Twitch and starting stream."
  envsubst < /opt/obs/config/service.json.tmpl \
    > "$OBS_HOME/basic/profiles/ADanaLife/service.json"
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
