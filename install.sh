#!/bin/bash
set -e

REPO="yanmxa/gencode"
BINARY="gen"
INSTALL_DIR="/usr/local/bin"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

info() { echo -e "${GREEN}$1${NC}"; }
warn() { echo -e "${YELLOW}$1${NC}"; }
error() { echo -e "${RED}$1${NC}" >&2; exit 1; }

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) error "Unsupported architecture: $ARCH" ;;
esac

case "$OS" in
    darwin|linux) ;;
    *) error "Unsupported OS: $OS" ;;
esac

# Get latest version
info "Fetching latest version..."
VERSION=$(curl -sL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')
[ -z "$VERSION" ] && error "Failed to get latest version"

info "Installing gen v${VERSION} for ${OS}/${ARCH}..."

# Download and extract
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/v${VERSION}/gen_${OS}_${ARCH}.tar.gz"
TMP_DIR=$(mktemp -d)
trap "rm -rf $TMP_DIR" EXIT

curl -sL "$DOWNLOAD_URL" -o "$TMP_DIR/gen.tar.gz" || error "Download failed"
tar -xzf "$TMP_DIR/gen.tar.gz" -C "$TMP_DIR" || error "Extract failed"

# Install
if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP_DIR/$BINARY" "$INSTALL_DIR/"
else
    warn "Requires sudo to install to $INSTALL_DIR"
    sudo mv "$TMP_DIR/$BINARY" "$INSTALL_DIR/"
fi

chmod +x "$INSTALL_DIR/$BINARY"

info "✓ gen v${VERSION} installed to $INSTALL_DIR/$BINARY"
echo ""
echo "Get started:"
echo "  gen              # Interactive mode"
echo "  gen \"message\"    # Quick question"
