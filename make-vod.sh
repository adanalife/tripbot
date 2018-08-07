#!/usr/bin/env bash

# This script takes a directory of videos and streams them to Twitch
#
# I use it for streaming dashcam footage, and because of this I apply
# a filter to blur out some a portion of the screen.

#TODO: check if we have ffmpeg and fail gracefully
#TODO: the blur needs to not cover the E/W

if [[ $# -eq 0 ]] ; then
  echo "Usage: $0 [dir containing vids]"
  exit 1
fi

VID_DIR="$1"

# use OUTPUT_DIR if set, otherwise use ./outputs
OUTPUT_DIR="${OUTPUT_DIR:-outputs}"

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
  -an \
  -c:v libx264 \
  -f mp4 \
  -r 60 \
  -t 7200 \
  $OUTPUT_DIR/$(date +%Y%m%d_%H%M%S).mp4

####################################################################
# safe to put old configs down here cause we wont ever get down here
####################################################################

exit 0

     movie=/Users/dmerrick/other_projects/stream/logo-fade-animation.gif[inner]; \
     [v][inner]overlay=70:70[v]" \
