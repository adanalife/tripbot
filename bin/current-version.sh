#!/usr/bin/env bash

git remote update 2>&1 >/dev/null
git describe --tags --abbrev=0 2>/dev/null

