#!/usr/bin/env bash

time \
  /root/make-vod.sh $INPUT_DIR | \
    tee -a $OUTPUT_DIR/make-vod.log

