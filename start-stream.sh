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
  -f concat \
  -safe 0 \
  -i <(for f in $VID_DIR/*.MP4; do echo "file '$f'"; done | sort -R) \
  -s 1920x1080 \
  -framerate 15 \
  -an \
  -c:v libx264 \
  -preset ultrafast \
  -pix_fmt yuv420p \
  -s 1920x1080 \
  -f flv "rtmp://live.twitch.tv/app/$STREAM_KEY"

  # -filter_complex \
  # "[0:v]crop=50:100:0:in_h-50,boxblur=10[fg]; \
  #  [0:v][fg]overlay=0:main_h-overlay_h[v]" \
  # -map "[v]" \
  #-c:v mpeg2video \

#  -vcodec copy \
