#!/bin/sh
set -e

REPO="GrayCodeAI/hawk"
BINARY="hawk"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
esac

LATEST=$(curl -sL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')
if [ -z "$LATEST" ]; then
  echo "Error: could not determine latest version"
  exit 1
fi

URL="https://github.com/$REPO/releases/download/v${LATEST}/${BINARY}_${LATEST}_${OS}_${ARCH}.tar.gz"
echo "Downloading hawk v${LATEST} for ${OS}/${ARCH}..."

TMP=$(mktemp -d)
curl -sL "$URL" | tar xz -C "$TMP"

INSTALL_DIR="/usr/local/bin"
if [ ! -w "$INSTALL_DIR" ]; then
  echo "Installing to $INSTALL_DIR (requires sudo)..."
  sudo mv "$TMP/$BINARY" "$INSTALL_DIR/"
else
  mv "$TMP/$BINARY" "$INSTALL_DIR/"
fi

rm -rf "$TMP"
echo "hawk v${LATEST} installed to $INSTALL_DIR/$BINARY"
