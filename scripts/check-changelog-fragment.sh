#!/usr/bin/env bash
# Pre-push guard: block a push whose branch adds no towncrier changelog fragment
# (the same check CI's changelog gate runs), so a missing fragment is caught
# locally instead of in CI. The authoritative gate stays CI + the skip-changelog
# PR label; this is a fast local pre-flight.
#
# Auto-skipped on develop/master/release/backmerge branches (they carry no
# fragment). Bypass for genuinely fragment-less work (CI-only tweaks, refactors,
# dep bumps):
#   SKIP_CHANGELOG=1 git push     (then add the skip-changelog label on the PR)
#   git push --no-verify          (skips all pre-push hooks)
set -euo pipefail

[ -n "${SKIP_CHANGELOG:-}" ] && exit 0

branch="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo HEAD)"
case "$branch" in
  develop | master | release/* | backmerge/*) exit 0 ;;
esac

base="origin/develop"
git rev-parse --verify --quiet "$base" >/dev/null 2>&1 || exit 0

if uvx towncrier check --config towncrier.toml --compare-with "$base"; then
  exit 0
fi

cat >&2 <<'MSG'

No changelog fragment found for this branch.
  Add one:  task changelog:add PR=<n> TYPE=<type>
  Or, if no entry is warranted (CI-only / refactor / dep bump):
    SKIP_CHANGELOG=1 git push   — and add the skip-changelog label on the PR.
MSG
exit 1
