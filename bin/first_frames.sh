#!/usr/bin/env bash

DIR=/Volumes/usbshare1

for file in $DIR/Dashcam/Movie/*.MP4; do
	no_extension=${file%.MP4}
	just_file=${no_extension##*/}
	ffmpeg -i $file -vframes 1 frames/$just_file.png
done
