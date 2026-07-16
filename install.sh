#!/bin/sh
set -e

# Versioned install target.
# By default the binary is installed under $HAWK_HOME/bin (default ~/.hawk/bin).
# Override with HAWK_HOME env var, or pass --prefix <dir> as the first flag.
HAWK_HOME="${HAWK_HOME:-$HOME/.hawk}"
if [ "$1" = "--prefix" ]; then
  HAWK_HOME="$2"
  shift 2
fi

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

LATEST=$(curl -fsSL --proto '=https' --tlsv1.2 "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')
if [ -z "$LATEST" ]; then
  echo "Error: could not determine latest version"
  exit 1
fi

ARCHIVE_NAME="${BINARY}_${LATEST}_${OS}_${ARCH}.${ARCHIVE_EXT}"
URL="https://github.com/$REPO/releases/download/v${LATEST}/${ARCHIVE_NAME}"
echo "Downloading hawk v${LATEST} for ${OS}/${ARCH}..."

TMP=$(mktemp -d)
ARCHIVE="$TMP/${ARCHIVE_NAME}"
curl -fsSL --proto '=https' --tlsv1.2 "$URL" -o "$ARCHIVE"

# TODO(release-eng): checksums.txt ships in the same release as the artifact,
# so it only protects against corruption — not a compromised release. Sign
# checksums.txt in goreleaser (cosign keyless or minisign) and verify the
# signature here when the verifier tool is available.
CHECKSUMS_URL="https://github.com/$REPO/releases/download/v${LATEST}/checksums.txt"
CHECKSUMS="$TMP/checksums.txt"
curl -fsSL --proto '=https' --tlsv1.2 "$CHECKSUMS_URL" -o "$CHECKSUMS"

if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL=$(sha256sum "$ARCHIVE" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL=$(shasum -a 256 "$ARCHIVE" | awk '{print $1}')
else
  echo "Error: no sha256sum or shasum found; cannot verify checksum"
  rm -rf "$TMP"
  exit 1
fi

# Exact-field match (not a regex grep): avoids '.' wildcards in the archive
# name matching other lines and producing a multi-line EXPECTED value.
EXPECTED=$(awk -v f="$ARCHIVE_NAME" '$2 == f { print $1 }' "$CHECKSUMS")
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

# Strip a leading "v" if the release tag was returned with one, so the
# versioned file name is the bare semver (e.g. 1.2.3).
VERSION=$(printf '%s' "$LATEST" | sed 's/^v//')

# --- Versioned install -------------------------------------------------------
# Adopted from SpaceXAI grok's postinstall.js. We never overwrite a binary in
# place. On macOS (and any codesigned platform) replacing a file that a running
# process has mmap'd invalidates the kernel's code-signature cache; the kernel
# then SIGKILLs that process. Installing into a per-version file and swapping
# the symlink means a running process keeps its open fd on the old inode and
# keeps running the previous version untouched — no SIGKILL, no disruption.
#
# The symlink is written to a temp name and then renamed into place so the
# rename is atomic; a racing process either sees the old or new link, never a
# half-written one.
#
# TODO(release-eng): checksums.txt ships in the same release as the artifact,
# so it only protects against corruption — not a compromised release. Sign
# checksums.txt in goreleaser (cosign keyless or minisign) and verify the
# signature here when the verifier tool is available. Versioned install is a
# prerequisite for safe in-place self-update tooling: once installs land at a
# stable versioned path + symlink, a future updater can swap the link without
# ever replacing a binary a running hawk has mmap'd (same SIGKILL rationale).
#
# Windows lacks reliable non-admin symlinks, so the launcher is a plain copy.

BINDIR="$HAWK_HOME/bin"
mkdir -p "$BINDIR"

if [ "$OS" = "windows" ]; then
  mv -f "$TMP/$BIN_NAME" "$BINDIR/hawk-$VERSION.exe"
  cp -f "$BINDIR/hawk-$VERSION.exe" "$BINDIR/hawk.exe"
  echo ""
  echo "Installed hawk v$VERSION to $BINDIR/hawk-$VERSION.exe"
  echo "Linked launcher: $BINDIR/hawk.exe"
else
  mv -f "$TMP/$BIN_NAME" "$BINDIR/hawk-$VERSION"
  ln -sf "hawk-$VERSION" "$BINDIR/hawk.tmp" \
    && mv -f "$BINDIR/hawk.tmp" "$BINDIR/hawk"
  echo ""
  echo "Installed hawk v$VERSION to $BINDIR/hawk-$VERSION (linked: $BINDIR/hawk)"
fi

rm -rf "$TMP"
echo ""
echo "Add $BINDIR to your PATH if it is not already, e.g."
echo "  export PATH=\"\$PATH:$BINDIR\""
echo ""
echo "Restart any running hawk sessions to pick up the new binary — the old"
echo "process keeps running the previous version until it is restarted."
