#!/bin/sh
# install.sh — download and install the latest (or pinned) obsidian-mcp release
# binary for the current OS/architecture, verifying its checksum first.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/tylern91/obsidian-mcp-server/main/install.sh | sh
#   curl -fsSL .../install.sh | sh -s -- v0.2.0
#   curl -fsSL .../install.sh | sh -s -- v0.2.0 /usr/local/bin
set -eu

REPO="tylern91/obsidian-mcp-server"
VERSION="${1:-latest}"
INSTALL_DIR="${2:-${HOME}/.local/bin}"

os="$(uname -s)"
arch="$(uname -m)"

case "$os" in
  Darwin) goos=darwin ;;
  Linux) goos=linux ;;
  *)
    echo "obsidian-mcp: unsupported OS '$os' — download a binary manually from" >&2
    echo "  https://github.com/${REPO}/releases" >&2
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

if [ "$VERSION" = "latest" ]; then
  echo "Resolving latest release …" >&2
  resolved="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep -m1 '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"
  if [ -z "$resolved" ]; then
    echo "obsidian-mcp: could not resolve the latest release tag" >&2
    exit 1
  fi
  VERSION="$resolved"
else
  case "$VERSION" in
    v*) ;;
    *) VERSION="v${VERSION}" ;;
  esac
fi

asset="obsidian-mcp-${VERSION}-${goos}-${goarch}"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

echo "Downloading ${asset}.tar.gz (${VERSION}) …" >&2
curl -fsSL -o "${tmp_dir}/${asset}.tar.gz" \
  "https://github.com/${REPO}/releases/download/${VERSION}/${asset}.tar.gz"
curl -fsSL -o "${tmp_dir}/${asset}.tar.gz.sha256" \
  "https://github.com/${REPO}/releases/download/${VERSION}/${asset}.tar.gz.sha256"

echo "Verifying checksum …" >&2
(cd "$tmp_dir" && shasum -a 256 -c "${asset}.tar.gz.sha256")

tar -xzf "${tmp_dir}/${asset}.tar.gz" -C "$tmp_dir"

mkdir -p "$INSTALL_DIR"
install -m 0755 "${tmp_dir}/${asset}/obsidian-mcp" "${INSTALL_DIR}/obsidian-mcp"

echo "Installed obsidian-mcp ${VERSION} to ${INSTALL_DIR}/obsidian-mcp" >&2
case ":$PATH:" in
  *":${INSTALL_DIR}:"*) ;;
  *) echo "Note: ${INSTALL_DIR} is not on your PATH — add it, or move the binary." >&2 ;;
esac
