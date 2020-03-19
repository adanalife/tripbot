#!/usr/bin/env bash

# this script generates a timelapse using a directory of screenshots

if [[ $# -eq 0 ]] ; then
    echo "Usage: $0 [output filename]"
    exit 0
fi

playlist_file=./playlist.txt

echo "generating playlist file (this will take a while)"
for f in gif-frames{,-odd}/*.png; do 
	echo "file '$(pwd)/$f'" >> $playlist_file.tmp
done

# merge the two dirs
sort -t\/ -k5 $playlist_file.tmp > $playlist_file
rm $playlist_file.tmp


echo "making the video"
time ffmpeg \
	-r 60 \
	-f concat -safe 0 -i $playlist_file \
	-vcodec libx264 -crf 25 -pix_fmt yuv420p \
	$1
