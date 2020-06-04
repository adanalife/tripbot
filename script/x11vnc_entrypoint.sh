#!/usr/bin/env bash

# this script is called by the container entrypint script
# it starts fluxbox (an X11 window manager) and kicks off
# other startup scripts

/usr/bin/fluxbox &

# fix for clipboard being passed through
#TODO: this has never been used, remove it?
#vncconfig -nowin &

if ls /opt/tripbot/script/startup_scripts/*.sh 1> /dev/null 2>&1; then
  for f in /opt/tripbot/script/startup_scripts/*.sh; do
    echo "running $f" >> /var/log/x11vnc_entrypoint.log
    bash "$f" || (echo "Error with $f: $?" >> /var/log/x11vnc_entrypoint.log) &
  done
fi
