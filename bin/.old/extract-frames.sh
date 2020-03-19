#!/usr/bin/env bash

# this script generates frames from specific moments in each video

DIR=/Volumes/usbshare1
loglevel=error
niceness=5

# trap ctrl-c and call ctrl_c()
trap ctrl_c INT
function ctrl_c() {
	echo "** Trapped CTRL-C"
	exit
}

for file in $DIR/Dashcam/Movie/*.MP4; do

	no_extension=${file%.MP4}
	just_file=${no_extension##*/}

	echo $just_file.MP4

	# [ -f "gif-frames/$just_file-000.png" ] && \ 
	# [ -f "gif-frames/$just_file-015.png" ] && \ 
	# [ -f "gif-frames/$just_file-030.png" ] && \ 
	# [ -f "gif-frames/$just_file-045.png" ] && \ 
	# [ -f "gif-frames/$just_file-100.png" ] && \ 
	# [ -f "gif-frames/$just_file-115.png" ] && \ 
	# [ -f "gif-frames/$just_file-130.png" ] && \ 
	# [ -f "gif-frames/$just_file-145.png" ] && \ 
	# [ -f "gif-frames/$just_file-200.png" ] && \ 
	# [ -f "gif-frames/$just_file-215.png" ] && \ 
	# [ -f "gif-frames/$just_file-230.png" ] && \ 
	# [ -f "gif-frames/$just_file-245.png" ] && \ 
	# echo "nothing to be done!" && \
	# continue

	# nice -n $niceness ffmpeg -n -hide_banner -loglevel $loglevel -i $file -vframes 1 -ss 00:00:15 gif-frames-odd/$just_file-015.png && \
	# nice -n $niceness ffmpeg -n -hide_banner -loglevel $loglevel -i $file -vframes 1 -ss 00:00:45 gif-frames-odd/$just_file-045.png && \
	# nice -n $niceness ffmpeg -n -hide_banner -loglevel $loglevel -i $file -vframes 1 -ss 00:01:15 gif-frames-odd/$just_file-115.png && \
	# nice -n $niceness ffmpeg -n -hide_banner -loglevel $loglevel -i $file -vframes 1 -ss 00:01:45 gif-frames-odd/$just_file-145.png && \
	# nice -n $niceness ffmpeg -n -hide_banner -loglevel $loglevel -i $file -vframes 1 -ss 00:02:15 gif-frames-odd/$just_file-215.png && \
	# nice -n $niceness ffmpeg -n -hide_banner -loglevel $loglevel -i $file -vframes 1 -ss 00:02:45 gif-frames-odd/$just_file-245.png && \
	# echo "done with odds!"

	# ffmpeg -i $file -vf fps=1/30 gif-frames/${just_file}-img%03d.jpg

	# nice -n $niceness ffmpeg -n -hide_banner -loglevel $loglevel -i $file -vframes 1 -ss 00:00:00 gif-frames/$just_file-000.png && \
	# nice -n $niceness ffmpeg -n -hide_banner -loglevel $loglevel -i $file -vframes 1 -ss 00:00:30 gif-frames/$just_file-030.png && \
	# nice -n $niceness ffmpeg -n -hide_banner -loglevel $loglevel -i $file -vframes 1 -ss 00:01:00 gif-frames/$just_file-100.png && \
	# nice -n $niceness ffmpeg -n -hide_banner -loglevel $loglevel -i $file -vframes 1 -ss 00:00:30 gif-frames/$just_file-130.png && \
	# nice -n $niceness ffmpeg -n -hide_banner -loglevel $loglevel -i $file -vframes 1 -ss 00:02:00 gif-frames/$just_file-200.png && \
	# nice -n $niceness ffmpeg -n -hide_banner -loglevel $loglevel -i $file -vframes 1 -ss 00:02:30 gif-frames/$just_file-230.png && \
	# echo "done with evens!"

	nice -n $niceness ffmpeg -n -hide_banner -loglevel $loglevel -i $file -vframes 1 -ss 00:00:00 gif-frames/$just_file-000.png 
	nice -n $niceness ffmpeg -n -hide_banner -loglevel $loglevel -i $file -vframes 1 -ss 00:00:15 gif-frames-odd/$just_file-015.png 
	nice -n $niceness ffmpeg -n -hide_banner -loglevel $loglevel -i $file -vframes 1 -ss 00:00:30 gif-frames/$just_file-030.png 
	nice -n $niceness ffmpeg -n -hide_banner -loglevel $loglevel -i $file -vframes 1 -ss 00:00:45 gif-frames-odd/$just_file-045.png 
	nice -n $niceness ffmpeg -n -hide_banner -loglevel $loglevel -i $file -vframes 1 -ss 00:01:00 gif-frames/$just_file-100.png 
	nice -n $niceness ffmpeg -n -hide_banner -loglevel $loglevel -i $file -vframes 1 -ss 00:01:15 gif-frames-odd/$just_file-115.png 
	nice -n $niceness ffmpeg -n -hide_banner -loglevel $loglevel -i $file -vframes 1 -ss 00:00:30 gif-frames/$just_file-130.png 
	nice -n $niceness ffmpeg -n -hide_banner -loglevel $loglevel -i $file -vframes 1 -ss 00:01:45 gif-frames-odd/$just_file-145.png 
	nice -n $niceness ffmpeg -n -hide_banner -loglevel $loglevel -i $file -vframes 1 -ss 00:02:00 gif-frames/$just_file-200.png 
	nice -n $niceness ffmpeg -n -hide_banner -loglevel $loglevel -i $file -vframes 1 -ss 00:02:15 gif-frames-odd/$just_file-215.png 
	nice -n $niceness ffmpeg -n -hide_banner -loglevel $loglevel -i $file -vframes 1 -ss 00:02:30 gif-frames/$just_file-230.png 
	nice -n $niceness ffmpeg -n -hide_banner -loglevel $loglevel -i $file -vframes 1 -ss 00:02:45 gif-frames-odd/$just_file-245.png 

	# echo "done with odds!"

	# ffmpeg -i $file -vf fps=1/30 gif-frames/${just_file}-img%03d.jpg

	# echo "done with evens!"
done

