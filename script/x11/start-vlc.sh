#!/usr/bin/env bash

# this script is executed as part of the x11 startup process

set -x

# check if X is running before starting
if ! xset q &>/dev/null; then
  echo "No X server at \$DISPLAY [$DISPLAY]" >&2
  sleep 1
  exit 1
fi

cleanup() {
  echo "Passing SIGTERM to vlc-server"
  kill -TERM "$vlc_pid" 2>/dev/null
}

trap cleanup SIGTERM

# sleep 5
# export DISPLAY=":0.0"

# hack VLC so we can run it as root
# c.p. https://unix.stackexchange.com/a/199422/202812
sed -i 's/geteuid/getppid/' /usr/bin/vlc

# hack to make fontconfig happy
#TODO: fix this
export FONTCONFIG_PATH=/etc/fonts

# compile vlc-server
cd /opt/tripbot || exit 2

# check if we have vlc-server compiled
if [[ ! -x "bin/vlc-server" ]]; then
  go build -o bin/vlc-server cmd/vlc-server/vlc-server.go 2>&1 | logger -t vlc-build
fi

# start vlc-server
bin/vlc-server 2>&1 | logger -t vlc-server &

vlc_pid=$!
wait "$vlc_pid"
