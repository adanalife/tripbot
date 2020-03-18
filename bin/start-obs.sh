#!/usr/bin/env bash

PID_FILE=run/OBS.pid
DATE="$(date "+%Y%m%d")"

echo "starting OBS..."

if [ "$(uname)" == 'Darwin' ]; then
  nice -n "-15" /Applications/OBS.app/Contents/MacOS/OBS -start "$@" >> log/obs-"${DATE}".log 2>&1 &
else
  # this is just for the hacky NVENC setup
  # export LD_LIBRARY_PATH="/home/dmerrick/other_projects/ffmpeg-nvenc/ffmpeg-nvenc/lib":$LD_LIBRARY_PATH
  # /home/dmerrick/other_projects/ffmpeg-nvenc/ffmpeg-nvenc/bin/obs -verbose -start "$@" >> log/obs-$(date "+%Y-%m-%d").log 2>&1 &
  # set DISPLAY if unset
  export DISPLAY="${DISPLAY:-:0}"
  if [ "$OBS_START_STREAMING" = "true" ] ; then
	/snap/bin/obs-studio --verbose --startstreaming "$@" >> log/obs-"${DATE}".log 2>&1 &
  else
	nice -n "-15" obs -start "$@" >> log/obs-"${DATE}".log 2>&1 &
  fi
fi

# save pid to file
PID=$!
echo $PID > $PID_FILE

echo "PID is: ${PID}"

# wait for the background job to end
wait $PID
rm $PID_FILE
