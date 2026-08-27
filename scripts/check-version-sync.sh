#!/usr/bin/env bash
# Asserts internal/version/version.go's Version const matches the top
# ## [X.Y.Z] heading in CHANGELOG.md. Run from the repo root.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION_FILE="$REPO_ROOT/internal/version/version.go"
CHANGELOG_FILE="$REPO_ROOT/CHANGELOG.md"

code_version=$(grep -oE 'const Version = "[^"]+"' "$VERSION_FILE" | grep -oE '[0-9]+\.[0-9]+\.[0-9]+')
if [[ -z "$code_version" ]]; then
    echo "error: could not parse Version const from $VERSION_FILE" >&2
    exit 1
fi

changelog_version=$(grep -m1 -oE '^## \[[0-9]+\.[0-9]+\.[0-9]+\]' "$CHANGELOG_FILE" | grep -oE '[0-9]+\.[0-9]+\.[0-9]+')
if [[ -z "$changelog_version" ]]; then
    echo "error: could not find a top ## [X.Y.Z] heading in $CHANGELOG_FILE" >&2
    exit 1
fi

if [[ "$code_version" != "$changelog_version" ]]; then
    echo "error: version mismatch — internal/version/version.go has $code_version, CHANGELOG.md top release heading has $changelog_version" >&2
    exit 1
fi

echo "ok: version.go and CHANGELOG.md agree on $code_version"
