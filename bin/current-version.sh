#!/usr/bin/env bash
# Called by pkg/chatbot/commands.go (versionCmd): prints latest git tag for !version.

git remote update >/dev/null 2>&1
git describe --tags --abbrev=0 2>/dev/null
