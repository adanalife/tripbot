#!/usr/bin/env bash

# This script takes a directory of videos and streams them to Twitch
#
# I use it for streaming dashcam footage, and because of this I apply
# a filter to blur out some a portion of the screen.

if [[ $# -eq 0 ]] ; then
  echo "Usage: $0 [stream key] [dir containing vids]"
  exit 1
fi

STREAM_KEY="$1"
VID_DIR="$2"

#TODO: check if we have ffmpeg and fail gracefully

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

# ffmpeg \
#   -hide_banner \
#   -safe 0 \
#   -f concat \
#   -i <(./smart-shuffle.rb $VID_DIR) \
#   -s 1920x1080 \
#   -maxrate 6000k \
#   -bufsize 4200k \
#   -framerate 15 \
#   -filter_complex \
#     "[0:v]crop=180:50:0:in_h-out_h,boxblur=10[fg]; \
#      [0:v][fg]overlay=0:main_h-overlay_h[v]" \
#   -map "[v]" \
#   -c:a copy \
#   -c:v libx264 \
#   -preset slow \
#   -pix_fmt yuv420p \
#   -s 1280x720 \
#   -r 30 \
#   -g 60 \
#   -crf 40 \
#   -f flv \
#   "rtmp://live.twitch.tv/app/$STREAM_KEY"

  # -preset slower \
  #-c:v mpeg2video \
  #-loglevel info \
#  -vcodec copy \
  # -x264-params "nal-hrd=cbr" \
  # -b:v 1M \
  # -minrate 1M \
  # -maxrate 1M \
  # -bufsize 2M \
  # -profile:v baseline \
  # -level 3.0 \
  # -movflags +faststart \
