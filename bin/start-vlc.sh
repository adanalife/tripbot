#!/usr/bin/env bash

PID_FILE=OBS/VLC.pid

echo "starting VLC..."

if [ $(uname) == 'Darwin' ]; then
  echo "TODO: build this out"
  exit 2
else
  export DISPLAY="${DISPLAY:-:0}"
  nice -n "-15" vlc >> log/vlc-$(date "+%Y-%m-%d").log 2>&1 &
fi

# save pid to file
PID=$!
echo $PID > $PID_FILE

echo "PID is: ${PID}"

# wait for the background job to end
wait $PID
rm $PID_FILE
