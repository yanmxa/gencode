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

usage() {
    echo "Usage: $0 [install|upgrade|uninstall]"
    echo ""
    echo "Commands:"
    echo "  install    Install gen (default)"
    echo "  upgrade    Upgrade to latest version"
    echo "  uninstall  Remove gen and config"
    exit 0
}

# Detect OS and architecture
detect_platform() {
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
}

get_latest_version() {
    curl -sL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/'
}

do_install() {
    detect_platform
    
    info "Fetching latest version..."
    VERSION=$(get_latest_version)
    [ -z "$VERSION" ] && error "Failed to get latest version"

    # Check if already installed
    if command -v "$BINARY" &>/dev/null; then
        CURRENT=$("$BINARY" version 2>/dev/null | awk '{print $3}' || echo "unknown")
        if [ "$CURRENT" = "$VERSION" ]; then
            info "✓ gen v${VERSION} is already installed"
            return
        fi
        info "Upgrading gen from v${CURRENT} to v${VERSION}..."
    else
        info "Installing gen v${VERSION} for ${OS}/${ARCH}..."
    fi

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
}

do_uninstall() {
    info "Uninstalling gen..."
    
    # Remove binary
    if [ -f "$INSTALL_DIR/$BINARY" ]; then
        if [ -w "$INSTALL_DIR" ]; then
            rm "$INSTALL_DIR/$BINARY"
        else
            warn "Requires sudo to remove from $INSTALL_DIR"
            sudo rm "$INSTALL_DIR/$BINARY"
        fi
        info "✓ Removed $INSTALL_DIR/$BINARY"
    else
        warn "Binary not found at $INSTALL_DIR/$BINARY"
    fi

    # Ask about config
    if [ -d "$HOME/.gen" ]; then
        echo -n "Remove config directory ~/.gen? [y/N] "
        read -r response
        if [[ "$response" =~ ^[Yy]$ ]]; then
            rm -rf "$HOME/.gen"
            info "✓ Removed ~/.gen"
        fi
    fi

    info "✓ Uninstall complete"
}

# Main
case "${1:-install}" in
    install|upgrade) do_install ;;
    uninstall|remove) do_uninstall ;;
    -h|--help|help) usage ;;
    *) error "Unknown command: $1" ;;
esac
