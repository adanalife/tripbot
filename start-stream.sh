#!/usr/bin/env bash

STREAM_KEY="live_225469317_4rrtBEFgOJ9ZxZSsjeHLScdOgWyZDj"
VID_DIR="/Volumes/Leeroy/Danas_Photos/Dashcam/DCIM/Movie"

BOX_OFFSET_X="30"
BOX_OFFSET_Y="50"

   # "[0:v]crop=200:200:$BOX_OFFSET_Y:$BOX_OFFSET_X,boxblur=10[fg]; \
   # #  [0:v][fg]overlay=$BOX_OFFSET_Y:$BOX_OFFSET_X[v]" \
  # "[0:v]crop=in_w-100:in_h-100:100:100,boxblur=10[fg]; \
  # [0:v][fg]overlay=100:100[v]" \
ffmpeg \
  -re \
  -f concat \
  -safe 0 \
  -i <(for f in $VID_DIR/*; do echo "file '$f'"; done | sort -R) \
  -filter_complex \
  "[0:v]crop=100:100:in_w:in_h-100,boxblur=10[fg]; \
   [0:v][fg]overlay=0:main_h-overlay_h[v]" \
  -map "[v]" \
  -s 1920x1200 \
  -framerate 15 \
  -an \
  -c:v libx264 \
  -preset ultrafast \
  -pix_fmt yuv420p \
  -s 1280x800 \
  -threads 2 \
  -f flv "rtmp://live.twitch.tv/app/$STREAM_KEY"
#  -crf 30 \


#  -vcodec copy \
