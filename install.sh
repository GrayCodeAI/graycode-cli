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

ARCHIVE_EXT="tar.gz"
BIN_NAME="$BINARY"

case "$OS" in
  mingw*|msys*|cygwin*)
    OS="windows"
    ARCHIVE_EXT="zip"
    BIN_NAME="${BINARY}.exe"
    ;;
esac

LATEST=$(curl -sL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')
if [ -z "$LATEST" ]; then
  echo "Error: could not determine latest version"
  exit 1
fi

ARCHIVE_NAME="${BINARY}_${LATEST}_${OS}_${ARCH}.${ARCHIVE_EXT}"
URL="https://github.com/$REPO/releases/download/v${LATEST}/${ARCHIVE_NAME}"
echo "Downloading hawk v${LATEST} for ${OS}/${ARCH}..."

TMP=$(mktemp -d)
ARCHIVE="$TMP/${ARCHIVE_NAME}"
curl -sL "$URL" -o "$ARCHIVE"

CHECKSUMS_URL="https://github.com/$REPO/releases/download/v${LATEST}/checksums.txt"
CHECKSUMS="$TMP/checksums.txt"
curl -sL "$CHECKSUMS_URL" -o "$CHECKSUMS"

if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL=$(sha256sum "$ARCHIVE" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL=$(shasum -a 256 "$ARCHIVE" | awk '{print $1}')
else
  echo "Error: no sha256sum or shasum found; cannot verify checksum"
  rm -rf "$TMP"
  exit 1
fi

EXPECTED=$(grep "${ARCHIVE_NAME}" "$CHECKSUMS" | awk '{print $1}')
if [ -z "$EXPECTED" ]; then
  echo "Error: checksum not found for ${ARCHIVE_NAME} in checksums.txt"
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

if [ "$OS" = "windows" ] && ! command -v unzip >/dev/null 2>&1; then
  echo "Error: unzip is required to install Windows release archives"
  rm -rf "$TMP"
  exit 1
fi

if [ "$OS" = "windows" ]; then
  unzip -q "$ARCHIVE" -d "$TMP"
else
  tar xz -C "$TMP" -f "$ARCHIVE"
fi

INSTALL_DIR="/usr/local/bin"
if [ ! -w "$INSTALL_DIR" ]; then
  echo "Installing to $INSTALL_DIR (requires sudo)..."
  sudo mv "$TMP/$BIN_NAME" "$INSTALL_DIR/"
else
  mv "$TMP/$BIN_NAME" "$INSTALL_DIR/"
fi

rm -rf "$TMP"
echo "hawk v${LATEST} installed to $INSTALL_DIR/$BIN_NAME"
