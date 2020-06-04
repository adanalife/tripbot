#!/usr/bin/env bash

# set up stream key and scenes
# RUN mkdir -p /root/.config/obs-studio/basic/profiles/Untitled/ /root/.config/obs-studio/basic/scenes/
# COPY config/basic.ini config/service.json /root/.config/obs-studio/basic/profiles/Untitled/
# COPY config/global.ini /root/.config/obs-studio/global.ini
# COPY config/Dashcam_Scenes.docker.json /root/.config/obs-studio/basic/scenes/Untitled.json
#TODO: instead of naming these both Untitled, edit ~/.config/obs-studio/global.ini

# copy configs over
cp -r /opt/tripbot/config/obs-studio /root/.config/

export FIXTHIS="pass this in via docker"

# set the streamkey from ENV var
sed -i "s/STREAMKEY/$FIXTHIS/" /root/.config/obs-studio/basic/profiles/Untitled/service.json

obs --startstreaming
