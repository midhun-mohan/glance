#!/bin/sh
set -e

REPO="midhun-mohan/glance"
BINARY="glance"
INSTALL_DIR="/usr/local/bin"

# Detect OS
OS="$(uname -s)"
case "$OS" in
  Darwin)  OS="darwin" ;;
  Linux)   OS="linux" ;;
  MINGW*|MSYS*|CYGWIN*) OS="windows" ;;
  *) echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)  ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

# Get latest version from GitHub API
echo "Fetching latest release..."
VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')"

if [ -z "$VERSION" ]; then
  echo "Failed to fetch latest version" >&2
  exit 1
fi

echo "Installing ${BINARY} v${VERSION} (${OS}/${ARCH})"

# Build download URL
ARCHIVE="${BINARY}_${OS}_${ARCH}"
if [ "$OS" = "windows" ]; then
  ARCHIVE="${ARCHIVE}.zip"
else
  ARCHIVE="${ARCHIVE}.tar.gz"
fi
URL="https://github.com/${REPO}/releases/download/v${VERSION}/${ARCHIVE}"

# Download to temp directory
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

echo "Downloading ${URL}..."
curl -fsSL "$URL" -o "${TMP_DIR}/${ARCHIVE}"

# Extract
echo "Extracting..."
if [ "$OS" = "windows" ]; then
  unzip -q "${TMP_DIR}/${ARCHIVE}" -d "$TMP_DIR"
else
  tar -xzf "${TMP_DIR}/${ARCHIVE}" -C "$TMP_DIR"
fi

# Install
if [ -w "$INSTALL_DIR" ]; then
  mv "${TMP_DIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
else
  echo "Installing to ${INSTALL_DIR} (requires sudo)..."
  sudo mv "${TMP_DIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
fi

chmod +x "${INSTALL_DIR}/${BINARY}"

echo ""
echo "glance v${VERSION} installed to ${INSTALL_DIR}/${BINARY}"
echo ""
echo "Prerequisites: gh CLI (https://cli.github.com)"
echo "  Run 'gh auth login' if you haven't already."
echo ""
echo "Usage: glance"
