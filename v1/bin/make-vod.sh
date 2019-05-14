#!/usr/bin/env bash

# This script takes a directory of videos and combines them
#
# I used it for streaming dashcam footage, and because of this I
# added a filter to blur out some a portion of the screen.

# make sure we have the right arguments
if [[ $# -eq 0 ]] ; then
  echo "Usage: $0 [dir containing vids]"
  exit 1
fi

VID_DIR="$1"

# make sure we have ffmpeg installed
if ! [ -x "$(command -v ffmpeg)" ]; then
  echo 'Error: ffmpeg is not installed.' >&2
  exit 2
fi

# use OUTPUT_DIR if set, otherwise use ./outputs
OUTPUT_DIR="${OUTPUT_DIR:-outputs}"
PLAYLISTS_DIR="${PLAYLISTS_DIR:-playlists}"

trap "exit 0" SIGINT SIGTERM

for playlist in $PLAYLISTS_DIR/*.txt; do

  # extract the playlist number
  n=$(echo $playlist | sed -e 's/[^0-9]//g')

  echo
  echo "Creating video${n} from playlist${n}"
  echo

  time ffmpeg \
    -hide_banner      `# reduce output` \
    -f concat -safe 0 `# combine files` \
    -i ${playlist} `# use playlists that were pre-generated` \
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
    \
    $OUTPUT_DIR/video${n}.mp4

  echo
  echo "Done with video${n}!"
  echo

done

