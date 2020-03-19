#!/usr/bin/env bash

# this script backs up the directory structure to another drive

# rsync -n -madPhi --delete /Volumes/Leeroy/Danas_Photos/Dashcam\ Scratchpad/Dashcam/ /Volumes/usbshare1/Dashcam/
rsync -madPhi --exclude *2018_0513_125617_019_opt.MP4 --exclude screencaps --delete /Volumes/usbshare1/Dashcam/ /Volumes/Leeroy/Danas_Photos/Dashcam\ Scratchpad/Dashcam/
