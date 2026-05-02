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
TARBALL="$TMP/${BINARY}_${LATEST}_${OS}_${ARCH}.tar.gz"
curl -sL "$URL" -o "$TARBALL"

CHECKSUMS_URL="https://github.com/$REPO/releases/download/v${LATEST}/checksums.txt"
CHECKSUMS="$TMP/checksums.txt"
curl -sL "$CHECKSUMS_URL" -o "$CHECKSUMS"

if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL=$(sha256sum "$TARBALL" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL=$(shasum -a 256 "$TARBALL" | awk '{print $1}')
else
  echo "Error: no sha256sum or shasum found; cannot verify checksum"
  rm -rf "$TMP"
  exit 1
fi

EXPECTED=$(grep "${BINARY}_${LATEST}_${OS}_${ARCH}.tar.gz" "$CHECKSUMS" | awk '{print $1}')
if [ -z "$EXPECTED" ]; then
  echo "Error: checksum not found for ${BINARY}_${LATEST}_${OS}_${ARCH}.tar.gz in checksums.txt"
  rm -rf "$TMP"
  exit 1
fi

if [ "$ACTUAL" != "$EXPECTED" ]; then
  echo "Error: checksum verification failed"
  echo "  expected: $EXPECTED"
  echo "  actual:   $ACTUAL"
  rm -rf "$TMP"
  exit 1
fi
echo "Checksum verified."

tar xz -C "$TMP" -f "$TARBALL"

INSTALL_DIR="/usr/local/bin"
if [ ! -w "$INSTALL_DIR" ]; then
  echo "Installing to $INSTALL_DIR (requires sudo)..."
  sudo mv "$TMP/$BINARY" "$INSTALL_DIR/"
else
  mv "$TMP/$BINARY" "$INSTALL_DIR/"
fi

rm -rf "$TMP"
echo "hawk v${LATEST} installed to $INSTALL_DIR/$BINARY"
