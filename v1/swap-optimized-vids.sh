#!/usr/bin/env bash


DASHCAM_DIR=$1

OPTIMIZED_DIR="${DASHCAM_DIR}/optimized"
UNOPTIMIZED_DIR="${DASHCAM_DIR}/unoptimized"
ALL_DIR="${DASHCAM_DIR}/_all"

echo $OPTIMIZED_DIR
echo $UNOPTIMIZED_DIR

for file in ${OPTIMIZED_DIR}/*.MP4; do
	dashStr="$(echo $file | sed -e 's!^.*/!!' -e 's/_opt//')"
	echo "mv $file ${ALL_DIR}/"
	echo "mv ${ALL_DIR}/${dashStr} ${UNOPTIMIZED_DIR}/"
done
