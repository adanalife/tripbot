#!/usr/bin/env bash

# This script takes a directory of videos and streams them to Twitch
#
# I use it for streaming dashcam footage, and because of this I apply
# a filter to blur out some a portion of the screen.

# make sure we have the right arguments
if [[ $# -eq 0 ]] ; then
  echo "Usage: $0 [stream key] [dir containing vids]"
  exit 1
fi

STREAM_KEY="$1"
VID_DIR="$2"

# make sure we have ffmpeg installed
if ! [ -x "$(command -v ffmpeg)" ]; then
  echo 'Error: ffmpeg is not installed.' >&2
  exit 2
fi

ffmpeg \
  -hide_banner      `# reduce output` \
  -f concat -safe 0 `# combine files` \
  -i <(./smart-shuffle.rb $VID_DIR) `# use script to generate input file` \
  \
  -filter_complex   `# cover the bottom left corner of the output` \
    "[0:v]crop=130:46:25:in_h-out_h-10,boxblur=10[fg]; \
     [0:v][fg]overlay=25:main_h-overlay_h-10[v] " -map "[v]" \
   \
  -s 1920x1080      `# output resolution` \
  -r 30             `# output FPS` \
  \
  -c:v libx264      `# output video codec` \
  -an               `# no output audio codec` \
  -f flv            `# output filetype` \
  -preset fast      `# speed up encoding ` \
  -crf 28           `# optimize the file` \
  -pix_fmt yuv420p  `# required for streaming?` \
  -x264-params "nal-hrd=cbr" `# try and force constant bit rate` \
  -minrate 850k -maxrate 850k -b:v 900k -bufsize 280k `# set bitrate controls` \
  -force_key_frames 'expr:gte(t,n_forced*2)' `# keyframes every two seconds` \
  "rtmp://live.twitch.tv/app/$STREAM_KEY"

