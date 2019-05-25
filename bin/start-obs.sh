#!/usr/bin/env bash

PID_FILE=OBS.pid

echo "starting OBS..."
nice -n "-15" /Applications/OBS.app/Contents/MacOS/OBS -start >> obs-$(date "+%Y-%m-%d").log 2>&1 &

# save pid to file
PID=$1
echo $PID > $PID_FILE

# wait for the background job to end
wait $PID
rm $PID_FILE
