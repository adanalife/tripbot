#!/usr/bin/env bash

# hack VLC so we can run it as root
# c.p. https://unix.stackexchange.com/a/199422/202812
sed -i 's/geteuid/getppid/' /usr/bin/vlc

# hack to make fontconfig happy
#TODO: fix this
export FONTCONFIG_PATH=/etc/fonts

# compile vlc-server
cd /opt/tripbot || exit 2
go build -o bin/vlc-server cmd/vlc-server/vlc-server.go | tee -a log/build-vlc.log 2>&1

# run vlc-server
bin/vlc-server | tee -a log/start-vlc.log 2>&1 &
