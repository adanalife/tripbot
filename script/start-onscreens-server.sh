#!/usr/bin/env bash

# Entrypoint for the onscreens-server supervisord program. Runs the
# compiled Go binary that owns onscreen state + serves the OBS browser
# source feeds and the chatbot-driven show/hide endpoints.

set -x

cleanup() {
  echo "Passing SIGTERM to onscreens-server"
  kill -TERM "$(cat /opt/data/run/onscreens-server.pid)" 2>/dev/null
}

trap cleanup SIGTERM

cd /opt/tripbot || exit 2

if [[ ! -x "bin/onscreens-server" ]]; then
  go build -o bin/onscreens-server cmd/onscreens-server/onscreens-server.go | logger -t onscreens-build 2>&1
fi

bin/onscreens-server | logger -t onscreens-server 2>&1
