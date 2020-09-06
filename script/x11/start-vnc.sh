#!/usr/bin/env bash

# start VNC server (Uses VNC_PASSWD Docker ENV variable)
mkdir -p "$HOME/.vnc" \
  && echo "$VNC_PASSWD" | vncpasswd -f > "$HOME/.vnc/passwd"

vncserver "$DISPLAY" -fg -Log *:syslog:100 -localhost no -nolisten -passwd "$HOME/.vnc/passwd"
