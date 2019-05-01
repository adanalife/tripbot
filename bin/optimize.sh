#!/usr/bin/env bash

file=$1

no_extension=${file%.MP4}
just_file=${no_extension##*/}

#ffmpeg -i $1 -preset slow -crf 21 -bufsize 10000k -maxrate 5000k -pix_fmt yuv420p -c:v libx264 -x264opts keyint=2:no-scenecut -s 1920x1080 -r 60 -an ${just_file}_opt.MP4
ffmpeg -i $1 -preset veryslow -crf 15 -bufsize 20000k -maxrate 10000k -pix_fmt yuv420p -c:v libx264 -x264opts keyint=2:no-scenecut -s 1920x1080 -r 60 -an ${just_file}_opt.MP4


# -preset slow
# -profile:v <profile>
# -sws_flags <scaler algorithm>
# -hls_list_size <number of playlist entries>
