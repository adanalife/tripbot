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

trap "exit 0" SIGINT SIGTERM

for i in `seq 0 100`; do

  echo
  echo "Working on playlist${i}.txt"
  echo

  ffmpeg \
    -hide_banner      `# reduce output` \
    -f concat -safe 0 `# combine files` \
    -i $OUTPUT_DIR/playlists/playlist${i}.txt `# use playlists that were pre-generated` \
    \
    -filter_complex   `# cover the bottom left corner of the output` \
      "[0:v]crop=130:46:25:in_h-out_h-10,boxblur=10[fg]; \
       [0:v][fg]overlay=25:main_h-overlay_h-10[v] " -map "[v]" \
    \
    -s 1920x1080      `# output resolution` \
    -r 60             `# output FPS` \
    \
    -c:v libx264      `# output video codec` \
    -an               `# no output audio codec` \
    -f mp4            `# output filetype` \
    -preset fast \
    \
    $OUTPUT_DIR/video${i}.mp4

  echo
  echo "Done with playlist${i}.txt"
  echo

done
####################################################################
# safe to put old configs down here cause we wont ever get down here
####################################################################

exit 0

    -t 7200           `# duration in seconds` \
     movie=/Users/dmerrick/other_projects/stream/logo-fade-animation.gif[inner]; \
     [v][inner]overlay=70:70[v]" \
