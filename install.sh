#!/bin/bash
set -e

REPO="yanmxa/gencode"
BINARY="gen"
INSTALL_DIR="/usr/local/bin"

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

case "$OS" in
    darwin|linux) ;;
    *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Get latest version
VERSION=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$VERSION" ]; then
    echo "Building from source..."

    # Check for Go
    if ! command -v go &> /dev/null; then
        echo "Go is required. Install from https://go.dev/dl/"
        exit 1
    fi

    TEMP_DIR=$(mktemp -d)
    cd "$TEMP_DIR"
    git clone --depth 1 https://github.com/$REPO.git
    cd gencode
    go build -o $BINARY ./cmd/gen

    echo "Installing to $INSTALL_DIR (may require sudo)..."
    sudo mv $BINARY $INSTALL_DIR/

    cd /
    rm -rf "$TEMP_DIR"
else
    # Download pre-built binary
    URL="https://github.com/$REPO/releases/download/$VERSION/${BINARY}_${OS}_${ARCH}.tar.gz"

    echo "Downloading $BINARY $VERSION..."
    TEMP_DIR=$(mktemp -d)
    curl -sL "$URL" | tar xz -C "$TEMP_DIR"

    echo "Installing to $INSTALL_DIR (may require sudo)..."
    sudo mv "$TEMP_DIR/$BINARY" $INSTALL_DIR/

    rm -rf "$TEMP_DIR"
fi

echo "Done! Run 'gen' to start."
