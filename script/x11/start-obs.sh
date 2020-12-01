#!/usr/bin/env bash

# this script is executed as part of the x11 startup process

# check if X is running before starting
if ! xset q &>/dev/null; then
  echo "No X server at \$DISPLAY [$DISPLAY]" >&2
  sleep 1
  exit 1
fi

cd /opt/tripbot/configs/obs-studio || exit 3

mkdir -p /root/.config/obs-studio/basic/profiles/Untitled/ /root/.config/obs-studio/basic/scenes/

# copy configs over
#TODO: instead of naming these both Untitled, rename
# these and edit ~/.config/obs-studio/global.ini
cp global.ini /root/.config/obs-studio/global.ini
cp basic.ini /root/.config/obs-studio/basic/profiles/Untitled
cp service.json /root/.config/obs-studio/basic/profiles/Untitled
cp Dashcam_Scenes.docker.json /root/.config/obs-studio/basic/scenes/Untitled.json

# set the streamkey from ENV var
sed -i "s/STREAMKEY/$STREAM_KEY/" /root/.config/obs-studio/basic/profiles/Untitled/service.json

obs --startstreaming --minimize-to-tray
