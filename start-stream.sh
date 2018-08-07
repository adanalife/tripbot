#!/usr/bin/env bash

# This script takes a directory of videos and streams them to Twitch
#
# I use it for streaming dashcam footage, and because of this I apply
# a filter to blur out some a portion of the screen.

#TODO: check if we have ffmpeg and fail gracefully
#TODO: the blur needs to not cover the E/W

if [[ $# -eq 0 ]] ; then
  echo "Usage: $0 [stream key] [dir containing vids]"
  exit 1
fi

STREAM_KEY="$1"
VID_DIR="$2"

ffmpeg \
  -hide_banner \
  -f concat \
  -safe 0 \
  -i <(./smart-shuffle.rb $VID_DIR) \
  -filter_complex \
    "[0:v]crop=130:46:25:in_h-out_h-10,boxblur=10[fg]; \
     [0:v][fg]overlay=25:main_h-overlay_h-10[v] "\
  -map "[v]" \
  -s 1920x1080 \
  -preset fast \
  -crf 28 \
  -an \
  -c:v libx264 \
  -x264-params "nal-hrd=cbr" \
  -pix_fmt yuv420p \
  -r 30 \
  -minrate 850k -maxrate 850k -b:v 900k -bufsize 280k \
  -force_key_frames 'expr:gte(t,n_forced*2)' \
  -f flv \
  "rtmp://live.twitch.tv/app/$STREAM_KEY"

####################################################################
# safe to put old configs down here cause we wont ever get down here
####################################################################

exit 0

# these are other options we could try/have tried

  -r 30 \
  -maxrate 12M \
  -bufsize 6M \
  -framerate 15 \
  -preset slow \
  -g 60 \
  -preset slower \
  -c:v mpeg2video \
  -loglevel info \
  -vcodec copy \
  -b:v 1M \
  -minrate 1M \
  -maxrate 1M \
  -bufsize 2M \
  -profile:v baseline \
  -level 3.0 \
  -movflags +faststart \
