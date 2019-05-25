#!/usr/bin/env bash

# this script takes a video file and generates an optimized-for-twitch version

# OBS outout from VLC plugin:
# info: [x264 encoder: 'simple_h264_stream'] preset: faster
# info: [x264 encoder: 'simple_h264_stream'] settings:
#         rate_control: CBR
#         bitrate:      4500
#         buffer size:  4500
#         crf:          0
#         fps_num:      60
#         fps_den:      1
#         width:        1920
#         height:       1080
#         keyint:       120

file=$1

no_extension=${file%.MP4}
just_file=${no_extension##*/}

# for info on keyint=120:no-scenecut
# https://video.stackexchange.com/a/24684
nice ffmpeg -n -i $1 \
  -preset faster \
  -crf 10 \
  -bufsize 20000k \
  -maxrate 10000k \
  -pix_fmt yuv420p \
  -c:v libx264 \
  -x264opts keyint=120:no-scenecut \
  -s 1920x1080 \
  -r 60 \
  -an \
  ${just_file}_opt.MP4

