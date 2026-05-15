#!/usr/bin/env sh

set -eu

REPO="dotcommander/glog"
VERSION="${1:-latest}"

usage() {
  cat <<'USAGE'
Usage: install.sh [VERSION]

Install the GLog CLI from GitHub releases.

Examples:
  curl -fsSL https://raw.githubusercontent.com/dotcommander/glog/main/scripts/install.sh | sh
  INSTALL_DIR="$HOME/bin" sh scripts/install.sh
  sh scripts/install.sh v0.1.0

Environment:
  INSTALL_DIR  Directory to install glog into. Defaults to /usr/local/bin when writable, otherwise ~/.local/bin.
USAGE
}

case "$VERSION" in
  -h|--help|help)
    usage
    exit 0
    ;;
esac

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

if [ ! -s "$ARCHIVE" ]; then
  printf "Downloaded archive not found: %s\n" "$ARCHIVE" >&2
  exit 1
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

case ":$PATH:" in
  *":$INSTALL_DIR:"*)
    printf "PATH: %s is already on PATH\n" "$INSTALL_DIR"
    ;;
  *)
    printf "PATH: %s is not on PATH yet\n" "$INSTALL_DIR"
    printf "Add it for this shell with:\n"
    printf "  export PATH=\"%s:\$PATH\"\n" "$INSTALL_DIR"
    ;;
esac

printf "\nVerify your install:\n"
printf "  glog doctor\n"
printf "\nStart your first local server:\n"
printf "  glog serve\n"
printf "\nIn another terminal, register and send a test log:\n"
printf "  glog host register --server http://localhost:6016 --name my-machine\n"
printf "  glog log \"hello from glog\"\n"
