#!/usr/bin/env bash

# this script finds the currently-playing video file

#TODO: error message if more than 1 result from search

lsof -p $(ps aux | grep -i OBS | grep -v grep | grep -v browser | awk '{print $2}') | grep MP4 | sed -e 's/^.*2018_/2018_/'
