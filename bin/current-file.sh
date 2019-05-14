#!/usr/bin/env bash

# this script finds the currently-playing video file

lsof -p $(ps aux | grep -i OBS | grep -v grep | awk '{print $2}') | grep MP4 | sed -e 's/^.*2018_/2018_/'
