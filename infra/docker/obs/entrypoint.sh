#!/usr/bin/env bash
# Called by infra/docker/obs/Dockerfile{,.arm64} (ENTRYPOINT via tini):
# OBS container entrypoint — seeds OBS config, starts Xvfb/fluxbox/x11vnc,
# launches OBS, and runs the hourly browser-source refresh loop.
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

# Stream encoder selection. Default obs_x264 (software) keeps stage-1 /
# local dev / Mac k3d working without /dev/dri exposed. Override via
# OBS_STREAM_ENCODER=ffmpeg_vaapi_tex (k8s configmap in prod-1) to use
# the host's Intel iGPU for hardware H.264 encode. OBS Simple Output
# mode silently falls back to x264 for VAAPI encoder values, so we run
# in Advanced Output mode (basic.ini.tmpl) and ship a per-encoder
# streamEncoder.json profile below.
#
# Note: Advanced Output's [AdvOut] Encoder field reads the literal
# encoder ID — no friendly-name aliases like Simple Output had. The
# x264 plugin registers as "obs_x264", so that's the string we write.
export OBS_STREAM_ENCODER="${OBS_STREAM_ENCODER:-obs_x264}"
echo "OBS stream encoder: ${OBS_STREAM_ENCODER}"

# OBS 32 split the legacy global.ini in two: app-level settings stayed in
# global.ini (BrowserHWAccel etc.) and user-preference settings moved to
# user.ini. Seed both so OBS sees a complete config and never prompts about
# migration.
cp /opt/obs/config/global.ini "$OBS_HOME/global.ini"
cp /opt/obs/config/user.ini   "$OBS_HOME/user.ini"
envsubst < /opt/obs/config/basic.ini.tmpl > "$OBS_HOME/basic/profiles/ADanaLife/basic.ini"
envsubst < /opt/obs/config/Tripbot.json.tmpl > "$OBS_HOME/basic/scenes/Tripbot.json"

# Advanced Output mode reads encoder-specific settings from streamEncoder.json
# in the profile dir. VAAPI's keys (vaapi_device, integer profile) don't
# overlap with x264's, so we case on OBS_STREAM_ENCODER to ship the right
# shape. Keep Twitch-friendly defaults: CBR, 2s keyframe interval, no B-frames.
case "${OBS_STREAM_ENCODER}" in
  ffmpeg_vaapi_tex)
    # profile=100 == AV_PROFILE_H264_HIGH (libavcodec). vaapi_device picks the
    # iGPU's renderD128 node — the only DRM node the Intel device plugin
    # exposes inside the pod.
    cat > "$OBS_HOME/basic/profiles/ADanaLife/streamEncoder.json" <<EOF
{
    "bf": 0,
    "bitrate": ${OBS_VIDEO_BITRATE},
    "keyint_sec": 2,
    "profile": 100,
    "rate_control": "CBR",
    "vaapi_device": "/dev/dri/renderD128"
}
EOF
    ;;
  *)
    # x264 (and any other software encoders) — profile is a string here.
    cat > "$OBS_HOME/basic/profiles/ADanaLife/streamEncoder.json" <<EOF
{
    "bitrate": ${OBS_VIDEO_BITRATE},
    "keyint_sec": 2,
    "preset": "${OBS_ENCODER_PRESET}",
    "profile": "high",
    "rate_control": "CBR"
}
EOF
    ;;
esac

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

# Hourly refresh of every browser source via the local obs-websocket port,
# working around the CEF per-frame memory leak that otherwise OOMs OBS at
# the 3Gi pod limit overnight. PR #555 capped browser-source render rate
# to slow the bleed; this loop drops accumulated render state on a fixed
# cycle so RSS stays bounded across multi-day uptimes.
(
  while sleep 3600; do
    OBS_WEBSOCKET_HOST=localhost OBS_WEBSOCKET_PORT=4455 \
      timeout 60 /opt/obs/venv/bin/python /opt/obs/bin/obs-browser-refresh \
      || echo "[browser-refresh] failed (will retry next cycle)" >&2
  done
) &

exec obs "${obs_args[@]}"
