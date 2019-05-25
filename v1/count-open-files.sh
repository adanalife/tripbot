#!/usr/bin/env bash

lsof -p $(ps aux | grep "[e]xe/make-map" | awk '{print $2}') | wc -l
