#!/usr/bin/env bash

# Entrypoint for the vlc-server supervisord program. Runs the compiled
# Go binary; libvlc handles playback headlessly (--vout dummy) and
# streams RTSP for OBS to consume.

set -x

cleanup() {
  echo "Passing SIGTERM to vlc-server"
  kill -TERM "$(cat /opt/data/run/vlc-server.pid)" 2>/dev/null
}

trap cleanup SIGTERM

cd /opt/tripbot || exit 2

# check if we have vlc-server compiled
if [[ ! -x "bin/vlc-server" ]]; then
  go build -o bin/vlc-server cmd/vlc-server/vlc-server.go | logger -t vlc-build 2>&1
fi

# start vlc-server
bin/vlc-server | logger -t vlc-server 2>&1
