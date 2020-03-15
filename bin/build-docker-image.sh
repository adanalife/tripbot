#!/usr/bin/env bash

#TODO: if cmd/tripbot/Dockerfile doesnt exist, exit early

docker build -t tripbot:latest . -f cmd/tripbot/Dockerfile
