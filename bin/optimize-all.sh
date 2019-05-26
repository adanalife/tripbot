#!/usr/bin/env bash

if [[ $# -eq 0 ]] ; then
  echo "USAGE: $0 dashcam-dir"
  exit 0
fi

DASHCAM_DIR=$1
SCRIPTPATH="$( cd "$(dirname "$0")" ; pwd -P )"

# trap ctrl-c and call ctrl_c()
trap ctrl_c INT

function ctrl_c() {
  echo "** Caught CTRL-C"
  exit 1
}

cd "${DASHCAM_DIR}/optimized"

for file in ${DASHCAM_DIR}/_all/*.MP4; do
  # skip this file if it's already optimized
  if [[ "$file" =~ _opt ]]; then
    echo "skipping $file because it's already optimized"
    continue
  fi
  #TODO: skip the file if it already exists in output dir
  $SCRIPTPATH/optimize-vid.sh $file
done

# move back to where we were
cd -

