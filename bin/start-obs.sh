#!/usr/bin/env bash

PID_FILE=OBS/OBS.pid

echo "starting OBS..."
#TODO: should be --startstreaming

if [ $(uname) == 'Darwin' ]; then
  nice -n "-15" /Applications/OBS.app/Contents/MacOS/OBS -start "$@" >> log/obs-$(date "+%Y-%m-%d").log 2>&1 &
else
  # this is just for the hacky NVENC setup
  export LD_LIBRARY_PATH="/home/dmerrick/other_projects/ffmpeg-nvenc/ffmpeg-nvenc/lib":$LD_LIBRARY_PATH
  nice -n "-15" /home/dmerrick/other_projects/ffmpeg-nvenc/ffmpeg-nvenc/bin/obs -start "$@" >> log/obs-$(date "+%Y-%m-%d").log 2>&1 &
fi

# save pid to file
PID=$!
echo $PID > $PID_FILE

echo "PID is: ${PID}"

# wait for the background job to end
wait $PID
rm $PID_FILE
