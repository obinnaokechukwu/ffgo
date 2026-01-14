#!/bin/bash
#
# build.sh - Build the ffshim library for ffgo
#
# This script builds the C shim library that provides wrappers for
# FFmpeg functionality that purego cannot handle directly.
#
# Usage:
#   ./build.sh              # Build for current platform
#   ./build.sh install      # Build and install to /usr/local/lib
#   ./build.sh clean        # Remove build artifacts

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Detect platform
OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
    Linux)
        SHARED_EXT=".so"
        SHARED_FLAGS="-shared -fPIC"
        INSTALL_DIR="/usr/local/lib"
        OUTPUT="libffshim${SHARED_EXT}"
        ;;
    Darwin)
        SHARED_EXT=".dylib"
        SHARED_FLAGS="-shared -fPIC"
        INSTALL_DIR="/usr/local/lib"
        OUTPUT="libffshim${SHARED_EXT}"
        ;;
    MINGW*|MSYS*|CYGWIN*)
        SHARED_EXT=".dll"
        SHARED_FLAGS="-shared"
        INSTALL_DIR=""
        OUTPUT="ffshim${SHARED_EXT}"
        ;;
    *)
        echo "Unsupported OS: $OS"
        exit 1
        ;;
esac

# Check for pkg-config
if ! command -v pkg-config &> /dev/null; then
    echo "Error: pkg-config not found. Please install it."
    exit 1
fi

# Check for FFmpeg development libraries
if ! pkg-config --exists libavutil libavcodec libavformat; then
    echo "Error: FFmpeg development libraries not found."
    echo "Please install FFmpeg development packages:"
    echo "  Ubuntu/Debian: sudo apt install libavcodec-dev libavformat-dev libavutil-dev"
    echo "  macOS:         brew install ffmpeg"
    echo "  Fedora:        sudo dnf install ffmpeg-devel"
    exit 1
fi

# Get compiler flags from pkg-config
CFLAGS="$(pkg-config --cflags libavutil libavcodec libavformat) -Wall -Wextra -O2"
LDFLAGS="$(pkg-config --libs libavutil libavcodec libavformat)"

build() {
    echo "Building ${OUTPUT}..."
    echo "  OS: $OS"
    echo "  ARCH: $ARCH"
    echo "  CFLAGS: $CFLAGS"
    echo "  LDFLAGS: $LDFLAGS"

    # Choose compiler
    if command -v clang &> /dev/null; then
        CC=clang
    elif command -v gcc &> /dev/null; then
        CC=gcc
    else
        echo "Error: No C compiler found. Please install gcc or clang."
        exit 1
    fi

    # Build
    $CC $SHARED_FLAGS $CFLAGS -o "$OUTPUT" ffshim.c $LDFLAGS

    echo "Built: $OUTPUT"
    echo "Size: $(ls -lh "$OUTPUT" | awk '{print $5}')"
}

install_lib() {
    if [ -z "$INSTALL_DIR" ]; then
        echo "Install not supported on this platform."
        echo "Please copy $OUTPUT to your desired location."
        exit 0
    fi

    if [ ! -f "$OUTPUT" ]; then
        build
    fi

    echo "Installing to $INSTALL_DIR..."

    if [ -w "$INSTALL_DIR" ]; then
        cp "$OUTPUT" "$INSTALL_DIR/"
    else
        echo "Need sudo to install to $INSTALL_DIR"
        sudo cp "$OUTPUT" "$INSTALL_DIR/"
    fi

    # Update library cache on Linux
    if [ "$OS" = "Linux" ]; then
        sudo ldconfig 2>/dev/null || true
    fi

    echo "Installed: $INSTALL_DIR/$OUTPUT"
}

clean() {
    echo "Cleaning..."
    rm -f libffshim.so libffshim.dylib ffshim.dll
    echo "Done."
}

case "${1:-build}" in
    build)
        build
        ;;
    install)
        install_lib
        ;;
    clean)
        clean
        ;;
    *)
        echo "Usage: $0 [build|install|clean]"
        exit 1
        ;;
esac
