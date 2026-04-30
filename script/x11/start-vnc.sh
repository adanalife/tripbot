#!/usr/bin/env bash

# Start X virtual framebuffer + window manager + VNC server
# Password is set via VNC_PASSWD env var (default: 123456)

Xvfb "$DISPLAY" -screen 0 1920x1200x24 &
sleep 1

fluxbox &
sleep 1

exec x11vnc -display "$DISPLAY" -forever -shared -rfbport 5900 -passwd "${VNC_PASSWD:-123456}"
