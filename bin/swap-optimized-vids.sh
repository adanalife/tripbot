#!/usr/bin/env bash

# this script looks for optimized versions of videos and swaps them in

if [[ $# -eq 0 ]] ; then
  echo "USAGE: $0 dashcam-dir"
  exit 0
fi

DASHCAM_DIR=$1

# set up important directories
OPTIMIZED_DIR="${DASHCAM_DIR}/optimized"
UNOPTIMIZED_DIR="${DASHCAM_DIR}/unoptimized"
ALL_DIR="${DASHCAM_DIR}/_all"

echo $OPTIMIZED_DIR
echo $UNOPTIMIZED_DIR


for file in ${ALL_DIR}/*; do
        # remove everything up to the slash and the extension
	slug="$(echo $file | sed -e 's!^.*/!!' -e 's/\.MP4//')"
	optimized="${OPTIMIZED_DIR}/${slug}_opt.MP4"

	if [ -e $optimized ]; then
		echo "optimized version exists, swapping $slug"
		# set -ex
		# mv $file ${UNOPTIMIZED_DIR}/
		# mv $optimized ${ALL_DIR}/
		# set +ex
		echo "mv $file ${UNOPTIMIZED_DIR}/"
		echo "mv $optimized ${ALL_DIR}/"
	else
		# echo "no optimized candidate for $slug, moving on"
		true
	fi
done

