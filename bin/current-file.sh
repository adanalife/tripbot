#!/usr/bin/env bash

# this script finds the currently-playing video file

# SCRIPTPATH="$( cd "$(dirname "$0")" ; pwd -P )"
# lsof -p $(cat ${SCRIPTPATH}/../run/OBS.pid) | grep -i '\.MP4' | sed -e 's/^.*2018_/2018_/'

# echo "2018_0522_201919_010_opt.MP4"
# ssh void -- "cd other_projects/danalol-stream && /Users/dmerrick/other_projects/danalol-stream/bin/current-file.sh"

#TODO: better explain why we use VLC here
if [[ "$OSTYPE" == "linux-gnu" ]]; then
  PIDFILE="run/VLC.pid"
elif [[ "$OSTYPE" == "darwin"* ]]; then
  PIDFILE="run/OBS.pid"
fi


#TODO: check for presence of file here
if [ ! -f $PIDFILE ]; then
  echo "Pidfile not found. Is OBS(OS X)/VLC(linux) running??"
  exit 2
fi

output=$(lsof -p $(cat $PIDFILE) 2>/dev/null)
if [ $? -eq 0 ]; then
  #TODO: error message if more than 1 result from search
  echo $output | grep -i '\.MP4' | sed -e 's/^.*2018_/2018_/' -e 's/MP4.*/MP4/'
else
  echo "No MP4s were found in lsof output. Check that the PID is correct and that OBS is playing videos"
  exit 3
fi
