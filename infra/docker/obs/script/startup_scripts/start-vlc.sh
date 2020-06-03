#!/usr/bin/env bash

cd /opt/tripbot
go build -o bin/vlc-server cmd/vlc-server/vlc-server.go
bin/vlc-server
