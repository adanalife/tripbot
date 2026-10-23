#!/usr/bin/env bash
# Rename one towncrier placeholder fragment to the PR it belongs to, and print
# the name it landed under.
#
#   scripts/number-fragment.sh 186 changelog.d/+keyboard.fixed.md
#   -> changelog.d/186.fixed.md
#
# Called by number-fragments.sh, which finds each placeholder's PR at release
# time; kept separate so the naming has its own self-check
# (number-fragment-test.sh).
set -euo pipefail

pr=$1
file=$2

base="$(basename "$file" .md)"  # +slug.fixed
type="${base##*.}"              # fixed
# `towncrier create` de-duplicates its own output by appending a counter before
# .md, so a second +behind.behind.md arrives as +behind.behind.1.md and the last
# segment is that counter rather than the type. Read as the type, it produces
# <PR>.1.md — a name towncrier rejects.
case "$type" in
  *[!0-9]*) ;;                        # a real type, not a counter
  *) stem="${base%.*}"; type="${stem##*.}" ;;
esac

# towncrier reads <issue>.<type>.<counter>.md, so repeat types in one PR get a
# counter rather than clobbering the first fragment.
target="changelog.d/${pr}.${type}.md"
n=0
while [ -e "$target" ]; do
  n=$((n + 1))
  target="changelog.d/${pr}.${type}.${n}.md"
done

git mv "$file" "$target"
printf '%s\n' "$target"
