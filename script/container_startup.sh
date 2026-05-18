#!/bin/bash
# Called by infra/docker/vlc/Dockerfile (ENTRYPOINT): VLC container entrypoint.
# OBS runs in its own container with its own entrypoint (see infra/docker/obs/entrypoint.sh).
#
# Static config files (supervisord program defs, fluxbox startup) are baked
# into the image at build time — see infra/docker/vlc/config/ and the COPY
# directives in infra/docker/vlc/Dockerfile.

#TODO: set background in /etc/X11/fluxbox/overlay

# don't ipen vncconfig on startup
sed -i '/vncconfig/s/^/#/' /etc/X11/Xvnc-session

mkdir -p "$XDG_RUNTIME_DIR"
chmod 0700 "$XDG_RUNTIME_DIR"

mkdir -p /opt/data/run
touch /opt/data/run/{left,right}-message.txt

cleanup() {
  echo "Gracefully stopping supervisor"
  supervisorctl stop all
  kill -TERM "$supervisor_pid" 2>/dev/null
  exit 3
}

trap cleanup SIGTERM

nohup supervisord --nodaemon -c /etc/supervisor/supervisord.conf 2>&1 | logger -t supervisor-init &
supervisor_pid=$!

# grc adds color to the output
grc -- tail -F /var/log/syslog
