#!/usr/bin/env bash

STREAM_KEY="$1"
VID_DIR="$2"

ffmpeg \
  -hide_banner \
  -f concat \
  -safe 0 \
  -i <(for f in $VID_DIR/*.MP4; do echo "file '$f'"; done) \
  -s 1920x1080 \
  -maxrate 6000k \
  -bufsize 4200k \
  -framerate 15 \
  -filter_complex \
  "[0:v]crop=180:50:0:in_h-50,boxblur=10[fg]; \
   [0:v][fg]overlay=0:main_h-overlay_h[v]" \
  -map "[v]" \
  -c:a copy \
  -c:v libx264 \
  -preset ultrafast \
  -pix_fmt yuv420p \
  -s 1280x720 \
  -g 120 \
  -crf 40 \
  -f flv \
  "rtmp://live.twitch.tv/app/$STREAM_KEY"

  #-c:v mpeg2video \
  #-loglevel info \
#  -vcodec copy \
