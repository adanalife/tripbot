#!/usr/bin/env bash

# ls -R /Volumes/Leeroy/Danas_Photos/Dashcam\ Scratchpad/Dashcam/ | awk '
ls -R . | awk '
/:$/&&f{s=$0;f=0}
/:$/&&!f{sub(/:$/,"");s=$0;f=1;next}
NF&&f{ print s"/"$0 }'
