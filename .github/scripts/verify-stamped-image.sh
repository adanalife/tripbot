#!/usr/bin/env bash
# Called by .github/workflows/release.yml (after each per-arch build/push):
# verifies a just-pushed release image has the expected version + SHA stamped
# into its binary. Pulls the image, runs it with --version (the image's
# entrypoint passes the flag through to the binary), and compares the two
# printed fields against expected values.
#
# Usage: verify-stamped-image.sh <image> <expected-version> <expected-sha>
#
# Fails (exit 1) if:
#   - the binary doesn't print two fields
#   - either field disagrees with the expected value
#   - the version reads as the literal string "dev" (build-arg not propagated)
set -euo pipefail

IMAGE="${1:?missing image arg}"
EXPECTED_VERSION="${2:?missing version arg}"
EXPECTED_SHA="${3:?missing sha arg}"

read -r GOT_VERSION GOT_SHA <<<"$(docker run --rm "$IMAGE" --version)"

echo "image:    $IMAGE"
echo "expected: version=$EXPECTED_VERSION sha=$EXPECTED_SHA"
echo "got:      version=$GOT_VERSION sha=$GOT_SHA"

fail=0
if [[ "$GOT_VERSION" == "dev" ]]; then
  echo "::error::release built with VERSION=dev — build-arg not propagated"
  fail=1
fi
if [[ "$GOT_VERSION" != "$EXPECTED_VERSION" ]]; then
  echo "::error::version mismatch in $IMAGE (got '$GOT_VERSION', want '$EXPECTED_VERSION')"
  fail=1
fi
if [[ "$GOT_SHA" != "$EXPECTED_SHA" ]]; then
  echo "::error::sha mismatch in $IMAGE (got '$GOT_SHA', want '$EXPECTED_SHA')"
  fail=1
fi

if (( fail )); then
  exit 1
fi

echo "OK"
