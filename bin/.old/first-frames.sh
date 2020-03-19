#!/usr/bin/env bash

# this script iterates over all of the videos
# and generates a full-res thumbnail image

DIR=/Volumes/usbshare1

for file in $DIR/Dashcam/Movie/*.MP4; do
	no_extension=${file%.MP4}
	just_file=${no_extension##*/}
	ffmpeg -i $file -vframes 1 frames/$just_file.png
done
