#!/usr/bin/env bash

git remote update >/dev/null 2>&1
git describe --tags --abbrev=0 2>/dev/null

