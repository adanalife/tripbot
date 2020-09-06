#!/usr/bin/env bash

# start VNC server (Uses VNC_PASSWD Docker ENV variable)
mkdir -p "$HOME/.vnc" \
  && echo "$VNC_PASSWD" | vncpasswd -f > "$HOME/.vnc/passwd"

vncserver "$DISPLAY" -localhost no -nolisten -rfbauth "$HOME/.vnc/passwd" -xstartup script/x11/start-x11.sh
