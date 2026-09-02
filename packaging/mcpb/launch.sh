#!/bin/sh
# Dispatches to the arch-specific binary bundled alongside this script.
# mcpb's platform_overrides only distinguish darwin/win32/linux, not CPU
# architecture, so darwin and linux both run through this wrapper; win32
# is amd64-only today and is pointed at its binary directly in manifest.json.
set -eu

dir="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)"

os="$(uname -s)"
arch="$(uname -m)"

case "$os" in
  Darwin) plat=darwin ;;
  Linux) plat=linux ;;
  *)
    echo "obsidian-mcp: unsupported platform '$os'" >&2
    exit 1
    ;;
esac

case "$arch" in
  arm64 | aarch64) goarch=arm64 ;;
  x86_64 | amd64) goarch=amd64 ;;
  *)
    echo "obsidian-mcp: unsupported architecture '$arch'" >&2
    exit 1
    ;;
esac

bin="$dir/obsidian-mcp-${plat}-${goarch}"
if [ ! -x "$bin" ]; then
  echo "obsidian-mcp: no bundled binary for ${plat}/${goarch} (expected $bin)" >&2
  exit 1
fi

exec "$bin" "$@"
