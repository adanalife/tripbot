#!/usr/bin/env bash
# Called by infra/docker/obs/Dockerfile{,.arm64} (ENTRYPOINT via tini):
# OBS container entrypoint — seeds OBS config from templates, then hands off
# to supervisor which manages sway/wayvnc/obs/browser-refresh.
#
# Display stack: sway (headless Wayland compositor on /dev/dri/card0) +
# wayvnc (VNC server, attaches to sway) + OBS as a Wayland-native Qt6
# client. Replaces the previous Xvfb+fluxbox+x11vnc trio so OBS's OpenGL
# composite hits the iGPU instead of Mesa llvmpipe.
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

# Streaming target platform. Default `twitch` preserves the original
# hardcoded behavior exactly (service "Twitch", server "auto" — OBS
# resolves "auto" via Twitch's ingest API at connect time). Set
# STREAM_PLATFORM=youtube (k8s configmap in the obs-youtube overlay) to
# point the same canvas/encoder at YouTube's RTMPS ingest. service.json.tmpl
# consumes OBS_STREAM_SERVICE / OBS_STREAM_SERVER via envsubst below.
case "${STREAM_PLATFORM:-twitch}" in
  youtube)
    export OBS_STREAM_SERVICE="YouTube - RTMPS"
    export OBS_STREAM_SERVER="rtmps://a.rtmps.youtube.com:443/live2"
    echo "OBS stream platform: youtube (YouTube - RTMPS)"
    ;;
  *)
    export OBS_STREAM_SERVICE="Twitch"
    export OBS_STREAM_SERVER="auto"
    echo "OBS stream platform: twitch (server auto)"
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

# Render service.json only when STREAM_KEY is set. start-obs.sh keys off
# this file's existence to decide whether to pass --startstreaming.
if [[ -n "${STREAM_KEY:-}" ]]; then
  echo "STREAM_KEY set; configuring ${OBS_STREAM_SERVICE} and starting stream."
  envsubst < /opt/obs/config/service.json.tmpl \
    > "$OBS_HOME/basic/profiles/ADanaLife/service.json"
else
  echo "STREAM_KEY not set; OBS will start idle. VNC into :5900 to inspect."
  rm -f "$OBS_HOME/basic/profiles/ADanaLife/service.json"
fi

# Shared Wayland runtime dir. start-sway.sh creates it with 0700; export
# it here so any debugging exec'd in the container picks it up too.
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/tmp/runtime-root}"
mkdir -p "$XDG_RUNTIME_DIR"
chmod 0700 "$XDG_RUNTIME_DIR"

# Render the wayvnc cfg into the per-pod tmpfs runtime dir. The template has
# no env vars to substitute — auth is off (wayvnc offers RFB "None" bound to
# the pod's localhost; access control lives at the traefik Ingress in front
# of noVNC), so the cert generation and VNC_USERNAME/VNC_PASSWD that used to
# render here are gone. envsubst is a passthrough copy now, kept so the cfg
# lands at the same spot as the rest of the per-pod runtime config.
envsubst < /opt/obs/config/wayvnc.cfg.tmpl > "$XDG_RUNTIME_DIR/wayvnc.cfg"

# Hand off to supervisord. It manages sway, wayvnc, obs, and the hourly
# browser-source refresh (with each program's start order + Wayland-socket
# dependency handled in script/start-*.sh).
exec supervisord -n -c /etc/supervisor/supervisord.conf
