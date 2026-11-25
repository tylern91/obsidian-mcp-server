#!/usr/bin/env bash
# check-stamp-calls.sh — guard the callers of stamp-changelog-refs.sh.
#
# The stamp script takes a BARE version (0.4.0) and derives the tag itself. Workflows hold
# v-prefixed versions, so every call must strip the prefix, and must never swallow a failure.
# Both mistakes shipped once (issue #76): finalize-notes passed "v0.4.0" and hid the error
# with `|| true`, so release bodies silently lost their commit links.
#
# Usage: check-stamp-calls.sh [workflow-dir]   (default: .github/workflows)
set -Eeuo pipefail

dir="${1:-.github/workflows}"
fail=0
calls=0

while IFS=: read -r file line text; do
  calls=$((calls + 1))
  if [[ "$text" != *'#v}'* ]]; then
    printf '%s:%s: stamp-changelog-refs.sh must be called with a bare version (use "${VAR#v}")\n' "$file" "$line" >&2
    fail=1
  fi
  if [[ "$text" == *'|| true'* ]]; then
    printf '%s:%s: stamp-changelog-refs.sh failure must not be swallowed (no "|| true")\n' "$file" "$line" >&2
    fail=1
  fi
done < <(grep -rnE 'bash +scripts/stamp-changelog-refs\.sh' "$dir" || true)

if [ "$calls" -eq 0 ]; then
  printf 'check-stamp-calls: no stamp-changelog-refs.sh calls found under %s\n' "$dir" >&2
  exit 1
fi
exit "$fail"
