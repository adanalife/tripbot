#!/usr/bin/env bash

if [[ $# -eq 0 ]] ; then
  echo "USAGE: $0 dashcam-dir"
  exit 0
fi

DASHCAM_DIR=$1
SCRIPTPATH="$( cd "$(dirname "$0")" ; pwd -P )"

cd "${DASHCAM_DIR}/optimized"

for file in ${DASHCAM_DIR}/_all/*.MP4; do
  $SCRIPTPATH/optimize-vid.sh $file
done

# move back to where we were
cd -

