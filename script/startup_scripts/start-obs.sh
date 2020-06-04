#!/usr/bin/env bash

# COPY config/basic.ini config/service.json /root/.config/obs-studio/basic/profiles/Untitled/
# COPY config/global.ini /root/.config/obs-studio/global.ini
# COPY config/Dashcam_Scenes.docker.json /root/.config/obs-studio/basic/scenes/Untitled.json

export FIXTHIS="pass this in via docker"

cd /opt/tripbot/configs/obs-studio

# copy configs over
#TODO: instead of naming these both Untitled, edit ~/.config/obs-studio/global.ini
cp basic.ini /root/.config/obs-studio/basic/profiles/Untitled
cp Dashcam_Scenes.linux.json /root/.config/obs-studio/basic/scenes/Untitled.json
cp global.ini /root/.config/obs-studio/global.ini

# set the streamkey from ENV var
sed -e "s/STREAMKEY/$FIXTHIS/" service.json /root/.config/obs-studio/basic/profiles/Untitled/service.json

#TODO: make it so we don't have to rename it here
cp /root/.config/obs-studio/Dashcam_Scenes.docker.json /root/.config/obs-studio/basic/scenes/Untitled.json

obs --startstreaming
