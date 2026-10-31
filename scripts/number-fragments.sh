#!/usr/bin/env bash
# Number every "+" placeholder fragment in changelog.d/ after the PR that added
# it, just before towncrier turns fragments into changelog lines. Fragments sit
# on main unnumbered until then: the squash commit that put each one there ends
# in "(#<PR>)", which is the number, and needs no branch to still exist.
#
# Needs full history (a fetch-depth: 0 checkout). A fragment whose adding
# commit carries no PR number is left as it is and publishes without a link.
set -euo pipefail
shopt -s nullglob

for f in changelog.d/+*.md; do
  subject="$(git log --diff-filter=A --format=%s -1 -- "$f")"
  pr="$(printf '%s' "$subject" | sed -nE 's/.*\(#([0-9]+)\)$/\1/p')"
  if [ -z "$pr" ]; then
    echo "::warning title=Unnumbered changelog fragment::${f} has no PR number in the commit that added it (\"${subject}\") — its entry will publish without a link."
    continue
  fi
  target="$("$(dirname "$0")/number-fragment.sh" "$pr" "$f")"
  echo "${f} -> ${target} (from #${pr})"
done
