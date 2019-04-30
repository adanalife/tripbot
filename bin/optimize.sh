#!/usr/bin/env bash

file=$1

no_extension=${file%.MP4}
just_file=${no_extension##*/}

ffmpeg -i $1 -c:v libx264 -x264opts keyint=2:no-scenecut -s 1920x1080 -r 60 -b:v 5000 -an ${just_file}_opt.MP4

# -profile:v <profile>
# -sws_flags <scaler algorithm>
# -hls_list_size <number of playlist entries>
