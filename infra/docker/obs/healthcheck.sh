#!/usr/bin/env bash
pgrep -x obs >/dev/null && DISPLAY=:0 xdpyinfo >/dev/null 2>&1
