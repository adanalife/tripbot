#!/usr/bin/env bash

# this script recursively lists all files in the current dir
# it is used to back up the current directory structure

if [[ $# -eq 0 ]] ; then
    echo "USAGE: $0 dashcam-dir"
    exit 0
fi

DASHCAM_DIR=$1

# we cd into the dir so we don't include the dir name in output
cd $DASHCAM_DIR

# https://stackoverflow.com/questions/1767384
ls -R . | awk '
/:$/&&f{s=$0;f=0}
/:$/&&!f{sub(/:$/,"");s=$0;f=1;next}
NF&&f{ print s"/"$0 }'

# go back to where we started
cd - >/dev/null
