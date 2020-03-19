#!/usr/bin/env bash

# this script finds videos that have been clipped (i.e. have an _a or _b suffix),
# and lists their "sister" files (the adjacent file that may also need to be clipped)

if [[ $# -eq 0 ]] ; then
  echo "USAGE: $0 dashcam-dir"
  exit 0
fi
DASHCAM_DIR=$1

cd $DASHCAM_DIR

# mkdir tmp

# https://stackoverflow.com/a/6282057
# this moves all of the "befores"
# for file in $(ls _all | grep -B1 _b | awk -F '\n' 'ln ~ /^$/ { ln = "matched"; print $1 } $1 ~ /^--$/ { ln = "" }' | grep -v _a | grep -v _c); do
#   # mv _all/$file tmp/
#   echo "mv _all/$file tmp/
# done

ls _all | grep -B1 _b | awk -F '\n' 'ln ~ /^$/ { ln = "matched"; print $1 } $1 ~ /^--$/ { ln = "" }' | grep -v _a | grep -v _c
ls _all | grep -A1 _a | grep -v '\-\-' | grep -v '_a' | grep -v _b

cd - > /dev/null
