#!/usr/bin/env bash

# set up stream key and scenes
# RUN mkdir -p /root/.config/obs-studio/basic/profiles/Untitled/ /root/.config/obs-studio/basic/scenes/
# COPY config/basic.ini config/service.json /root/.config/obs-studio/basic/profiles/Untitled/
# COPY config/global.ini /root/.config/obs-studio/global.ini
# COPY config/Dashcam_Scenes.docker.json /root/.config/obs-studio/basic/scenes/Untitled.json
#TODO: instead of naming these both Untitled, edit ~/.config/obs-studio/global.ini

cd /opt/tripbot

# copy OBS config files into place before starting
mkdir -p /root/.config/obs-studio/basic/profiles/Untitled/ /root/.config/obs-studio/basic/scenes/
cp config/OBS/basic.ini config/OBS/service.json /root/.config/obs-studio/basic/profiles/Untitled/
cp config/OBS/global.ini /root/.config/obs-studio/
cp config/OBS/Dashcam_Scenes.docker.json /root/.config/obs-studio/basic/scenes/Untitled.json

obs --startstreaming
