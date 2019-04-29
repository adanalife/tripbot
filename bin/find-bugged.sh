#!/usr/bin/env bash

find -s . -name "2018_*.MP4" -exec bash -c 'echo "{}"; ffmpeg -v error -i "{}" -map 0:1 -f null - >>error.log 2>&1' \;
