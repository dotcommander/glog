#!/usr/bin/env sh

set -eu

REPO="dotcommander/glog"
VERSION="${1:-latest}"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$OS" in
  linux) OS="linux" ;;
  darwin) OS="darwin" ;;
  *)
    printf "Unsupported OS: %s\n" "$OS" >&2
    exit 1
    ;;
esac

case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    printf "Unsupported architecture: %s\n" "$ARCH" >&2
    exit 1
    ;;
esac

if [ "${INSTALL_DIR:-}" = "" ]; then
  if [ -w "/usr/local/bin" ]; then
    INSTALL_DIR="/usr/local/bin"
  else
    INSTALL_DIR="$HOME/.local/bin"
  fi
fi

TMP_DIR="$(mktemp -d)"
ARCHIVE="$TMP_DIR/glog.tar.gz"

if [ "$VERSION" = "latest" ]; then
  URL="https://github.com/$REPO/releases/latest/download/glog_${OS}_${ARCH}.tar.gz"
else
  URL="https://github.com/$REPO/releases/download/$VERSION/glog_${OS}_${ARCH}.tar.gz"
fi

printf "Downloading %s\n" "$URL"
if ! curl -fsSL "$URL" -o "$ARCHIVE"; then
  if command -v gh >/dev/null 2>&1; then
    printf "Direct download failed, trying authenticated GitHub CLI download...\n"
    TAG="$VERSION"
    if [ "$TAG" = "latest" ]; then
      TAG="$(gh release list --repo "$REPO" --limit 1 --json tagName --jq '.[0].tagName')"
    fi
    gh release download "$TAG" --repo "$REPO" --pattern "glog_${OS}_${ARCH}.tar.gz" --dir "$TMP_DIR"
    ARCHIVE="$TMP_DIR/glog_${OS}_${ARCH}.tar.gz"
  else
    printf "Download failed and GitHub CLI not found for fallback auth.\n" >&2
    exit 1
  fi
fi

tar -xzf "$ARCHIVE" -C "$TMP_DIR"

mkdir -p "$INSTALL_DIR"
BIN_PATH="$(find "$TMP_DIR" -type f -name glog | head -n 1)"

if [ "$BIN_PATH" = "" ]; then
  printf "glog binary not found in archive\n" >&2
  exit 1
fi

install -m 0755 "$BIN_PATH" "$INSTALL_DIR/glog"

printf "Installed glog to %s/glog\n" "$INSTALL_DIR"
printf "Run: glog version\n"
