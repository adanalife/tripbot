#!/usr/bin/env bash
# Self-check for number-fragment.sh: the three names it has to get right.
# Run it directly — it works in a throwaway git repo and touches nothing else.
set -euo pipefail

script="$(cd "$(dirname "$0")" && pwd)/number-fragment.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cd "$tmp"
git init -q .
mkdir changelog.d

check() {
  [ "$1" = "$2" ] || { echo "want $2, got $1"; exit 1; }
}

add() { touch "$1"; git add "$1"; }

add changelog.d/+keyboard.fixed.md
check "$("$script" 186 changelog.d/+keyboard.fixed.md)" changelog.d/186.fixed.md

# A second fragment of the same type takes towncrier's counter rather than
# clobbering the first.
add changelog.d/+strip.fixed.md
check "$("$script" 186 changelog.d/+strip.fixed.md)" changelog.d/186.fixed.1.md

# towncrier's own de-duplicated name: the last segment is a counter, so the
# type is the one before it.
add changelog.d/+behind.behind.1.md
check "$("$script" 187 changelog.d/+behind.behind.1.md)" changelog.d/187.behind.md

# number-fragments.sh reads each placeholder's PR off the squash commit that
# added it, and leaves one with no number alone.
all="$(dirname "$script")/number-fragments.sh"
git -c user.name=t -c user.email=t@t commit -qm "seed"
add changelog.d/+voice-3f9a.new.md
git -c user.name=t -c user.email=t@t commit -qm "feat(intents): guess the state by voice (#275)"
add changelog.d/+direct-01ab.fixed.md
git -c user.name=t -c user.email=t@t commit -qm "fix: pushed with no PR"
"$all" > /dev/null
[ -e changelog.d/275.new.md ] || { echo "want changelog.d/275.new.md"; exit 1; }
[ -e changelog.d/+direct-01ab.fixed.md ] || { echo "a fragment with no PR number should stay put"; exit 1; }

echo "ok"
