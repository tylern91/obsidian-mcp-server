#!/usr/bin/env bash
# build-mcpb.sh <version> [output-dir]
#
# Downloads the release tarballs for <version> from GitHub, extracts each
# platform's binary into a single MCPB staging directory alongside
# packaging/mcpb/manifest.json.template and launch.sh, then packs the result
# into a .mcpb bundle via `npx @anthropic-ai/mcpb pack`.
#
# Requires: gh (authenticated), npx, tar, shasum.
#
# Usage:
#   ./scripts/build-mcpb.sh v0.3.0
#   ./scripts/build-mcpb.sh v0.3.0 ./dist

set -euo pipefail

VERSION="${1:-${VERSION:-}}"
if [[ -z "$VERSION" ]]; then
  echo "Usage: $0 <version> [output-dir]  (e.g. v0.3.0)" >&2
  exit 1
fi
VERSION="${VERSION#v}"
TAG="v${VERSION}"

REPO="tylern91/obsidian-mcp-server"
OUT_DIR="${2:-dist}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PACKAGING_DIR="${ROOT}/packaging/mcpb"

MCPB_CLI_VERSION="2.1.2"

STAGE_DIR="$(mktemp -d)"
trap 'rm -rf "$STAGE_DIR"' EXIT
SERVER_DIR="${STAGE_DIR}/server"
mkdir -p "$SERVER_DIR"

for target in darwin-arm64 darwin-amd64 linux-amd64 linux-arm64 windows-amd64; do
  asset="obsidian-mcp-${TAG}-${target}.tar.gz"
  echo "Fetching ${asset} …" >&2
  tmp_tar="${STAGE_DIR}/${asset}"
  gh release download "$TAG" -R "$REPO" -p "$asset" -O "$tmp_tar" --clobber

  extract_dir="${STAGE_DIR}/extract-${target}"
  mkdir -p "$extract_dir"
  tar -xzf "$tmp_tar" -C "$extract_dir"

  src_bin="${extract_dir}/obsidian-mcp-${TAG}-${target}/obsidian-mcp"
  if [[ "$target" == windows-* ]]; then
    src_bin="${src_bin}.exe"
    cp "$src_bin" "${SERVER_DIR}/obsidian-mcp-${target}"
  else
    cp "$src_bin" "${SERVER_DIR}/obsidian-mcp-${target}"
    chmod +x "${SERVER_DIR}/obsidian-mcp-${target}"
  fi
done

cp "${PACKAGING_DIR}/launch.sh" "${SERVER_DIR}/launch.sh"
chmod +x "${SERVER_DIR}/launch.sh"

sed "s|__VERSION__|${VERSION}|g" "${PACKAGING_DIR}/manifest.json.template" \
  > "${STAGE_DIR}/manifest.json"

mkdir -p "$OUT_DIR"
OUTPUT_MCPB="${OUT_DIR}/obsidian-mcp-${TAG}.mcpb"

echo "Packing ${OUTPUT_MCPB} …" >&2
npx --yes "@anthropic-ai/mcpb@${MCPB_CLI_VERSION}" pack "$STAGE_DIR" "$OUTPUT_MCPB"

shasum -a 256 "$OUTPUT_MCPB" | awk -v f="$(basename "$OUTPUT_MCPB")" '{print $1"  "f}' \
  > "${OUTPUT_MCPB}.sha256"

echo "Built ${OUTPUT_MCPB}" >&2
