#!/usr/bin/env bash

# this script backs up the directory structure to another drive

# rsync -n -madPhi --delete /Volumes/Leeroy/Danas_Photos/Dashcam\ Scratchpad/Dashcam/ /Volumes/usbshare1/Dashcam/
rsync -n -madPhi --delete /Volumes/usbshare1/Dashcam/ /Volumes/Leeroy/Danas_Photos/Dashcam\ Scratchpad/Dashcam/
