#!/bin/sh
# Entrypoint of the migrate image. `--version` answers the release workflow's
# stamp check with the same two fields the Go binaries print; anything else
# is a migrate invocation.
if [ "$1" = "--version" ]; then
  echo "$IMAGE_VERSION $IMAGE_SHA"
  exit 0
fi
exec migrate "$@"
