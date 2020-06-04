#!/usr/bin/env bash

#TODO: fail if STREAM_KEY isn't present

cd /opt/tripbot/configs/obs-studio

mkdir -p /root/.config/obs-studio/basic/profiles/Untitled/ /root/.config/obs-studio/basic/scenes/

# copy configs over
#TODO: instead of naming these both Untitled, edit ~/.config/obs-studio/global.ini
cp global.ini /root/.config/obs-studio/global.ini
cp basic.ini /root/.config/obs-studio/basic/profiles/Untitled
cp service.json /root/.config/obs-studio/basic/profiles/Untitled
cp Dashcam_Scenes.docker.json /root/.config/obs-studio/basic/scenes/Untitled.json

# set the streamkey from ENV var
sed -i "s/STREAMKEY/$STREAM_KEY/" /root/.config/obs-studio/basic/profiles/Untitled/service.json

obs --startstreaming
