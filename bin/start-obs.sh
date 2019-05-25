#!/usr/bin/env bash

PID_FILE=OBS/OBS.pid

echo "starting OBS..."
#TODO: should be --startstreaming
nice -n "-15" /Applications/OBS.app/Contents/MacOS/OBS -start >> log/obs-$(date "+%Y-%m-%d").log 2>&1 &

# save pid to file
PID=$!
echo $PID > $PID_FILE

echo "PID is: ${PID}"

# wait for the background job to end
wait $PID
rm $PID_FILE
