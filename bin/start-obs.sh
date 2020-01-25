#!/usr/bin/env bash

PID_FILE=OBS/OBS.pid

# this is just for the hacky NVENC setup
export LD_LIBRARY_PATH="/home/dmerrick/other_projects/ffmpeg-nvenc/ffmpeg-nvenc/lib":$LD_LIBRARY_PATH

echo "starting OBS..."
#TODO: should be --startstreaming
/home/dmerrick/other_projects/ffmpeg-nvenc/ffmpeg-nvenc/bin/obs "$@"
#nice -n "-15" /Applications/OBS.app/Contents/MacOS/OBS -start "$@" >> log/obs-$(date "+%Y-%m-%d").log 2>&1 &
nice -n "-15" ./obs -start "$@" >> log/obs-$(date "+%Y-%m-%d").log 2>&1 &

# save pid to file
PID=$!
echo $PID > $PID_FILE

echo "PID is: ${PID}"

# wait for the background job to end
wait $PID
rm $PID_FILE
