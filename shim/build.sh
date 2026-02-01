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
#   ./build.sh prebuilt     # Build and copy to prebuilt/<os>-<arch>/
#   ./build.sh help         # Show this help
#
# Environment variables:
#   CC          - C compiler (default: auto-detect clang/gcc)
#   PREFIX      - Installation prefix (default: /usr/local)
#   FFMPEG_DIR  - FFmpeg installation directory (for headers/libs)
#
# The shim is OPTIONAL - core ffgo functionality works without it.
# Only logging callbacks and some advanced features require the shim.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Configuration
PREFIX="${PREFIX:-/usr/local}"
CC="${CC:-}"

# Detect platform
OS="$(uname -s)"
ARCH="$(uname -m)"

# Normalize architecture names
case "$ARCH" in
    x86_64|amd64)
        ARCH_NORMALIZED="amd64"
        ;;
    aarch64|arm64)
        ARCH_NORMALIZED="arm64"
        ;;
    *)
        ARCH_NORMALIZED="$ARCH"
        ;;
esac

# Platform-specific settings
case "$OS" in
    Linux)
        SHARED_EXT=".so"
        SHARED_FLAGS="-shared -fPIC"
        INSTALL_DIR="$PREFIX/lib"
        OUTPUT="libffshim${SHARED_EXT}"
        OS_NORMALIZED="linux"
        ;;
    Darwin)
        SHARED_EXT=".dylib"
        SHARED_FLAGS="-shared -fPIC"
        INSTALL_DIR="$PREFIX/lib"
        OUTPUT="libffshim${SHARED_EXT}"
        OS_NORMALIZED="darwin"
        ;;
    MINGW*|MSYS*|CYGWIN*)
        SHARED_EXT=".dll"
        SHARED_FLAGS="-shared -Wl,--export-all-symbols"
        INSTALL_DIR=""
        OUTPUT="ffshim${SHARED_EXT}"
        OS_NORMALIZED="windows"
        ;;
    *)
        echo "Unsupported OS: $OS"
        exit 1
        ;;
esac

# Auto-detect compiler
detect_compiler() {
    if [ -n "$CC" ]; then
        return
    fi

    if command -v clang &> /dev/null; then
        CC=clang
    elif command -v gcc &> /dev/null; then
        CC=gcc
    else
        echo "Error: No C compiler found. Please install gcc or clang."
        exit 1
    fi
}

# Check for pkg-config
check_pkg_config() {
    if ! command -v pkg-config &> /dev/null; then
        echo "Error: pkg-config not found. Please install it."
        exit 1
    fi
}

# Check for FFmpeg development libraries
check_ffmpeg() {
    if ! pkg-config --exists libavutil libavcodec libavformat; then
        echo "Error: FFmpeg development libraries not found."
        echo ""
        echo "Please install FFmpeg development packages:"
        case "$OS" in
            Linux)
                if command -v apt-get &> /dev/null; then
                    echo "  sudo apt install libavcodec-dev libavformat-dev libavutil-dev"
                elif command -v dnf &> /dev/null; then
                    echo "  sudo dnf install ffmpeg-devel"
                elif command -v pacman &> /dev/null; then
                    echo "  sudo pacman -S ffmpeg"
                else
                    echo "  Install ffmpeg development libraries for your distribution"
                fi
                ;;
            Darwin)
                echo "  brew install ffmpeg"
                ;;
            *)
                echo "  Install FFmpeg development libraries"
                ;;
        esac
        exit 1
    fi
}

# Get compiler flags from pkg-config
get_flags() {
    PKG_LIBS="libavutil libavcodec libavformat"
    EXTRA_DEFINES=""

    # Optional: libavdevice (enables device enumeration helpers)
    if pkg-config --exists libavdevice 2>/dev/null; then
        PKG_LIBS="$PKG_LIBS libavdevice"
        EXTRA_DEFINES="-DFFSHIM_HAVE_AVDEVICE=1"
        echo "  libavdevice: found (device helpers enabled)"
    else
        echo "  libavdevice: not found (device helpers disabled)"
    fi

    CFLAGS="$(pkg-config --cflags $PKG_LIBS) -Wall -Wextra -O2 $EXTRA_DEFINES"
    LDFLAGS="$(pkg-config --libs $PKG_LIBS)"
}

build() {
    echo "Building ${OUTPUT}..."
    echo "  Platform: $OS_NORMALIZED-$ARCH_NORMALIZED"

    check_pkg_config
    check_ffmpeg
    detect_compiler
    get_flags

    echo "  Compiler: $CC"
    echo "  CFLAGS: $CFLAGS"
    echo "  LDFLAGS: $LDFLAGS"
    echo ""

    # Build
    $CC $SHARED_FLAGS $CFLAGS -o "$OUTPUT" ffshim.c $LDFLAGS

    echo "Built: $OUTPUT"
    echo "Size: $(ls -lh "$OUTPUT" | awk '{print $5}')"
    echo ""
    echo "To use the shim, either:"
    echo "  1. Install it:  ./build.sh install"
    echo "  2. Set FFGO_SHIM_DIR=$SCRIPT_DIR"
    case "$OS" in
        Linux)
            echo "  3. Add to LD_LIBRARY_PATH: export LD_LIBRARY_PATH=$SCRIPT_DIR:\$LD_LIBRARY_PATH"
            ;;
        Darwin)
            echo "  3. Add to DYLD_LIBRARY_PATH: export DYLD_LIBRARY_PATH=$SCRIPT_DIR:\$DYLD_LIBRARY_PATH"
            ;;
    esac
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

prebuilt() {
    # Build and copy to prebuilt directory for distribution
    if [ ! -f "$OUTPUT" ]; then
        build
    fi

    PREBUILT_DIR="prebuilt/${OS_NORMALIZED}-${ARCH_NORMALIZED}"
    mkdir -p "$PREBUILT_DIR"
    cp "$OUTPUT" "$PREBUILT_DIR/"
    echo "Copied to: $PREBUILT_DIR/$OUTPUT"
}

clean() {
    echo "Cleaning..."
    rm -f libffshim.so libffshim.dylib ffshim.dll
    echo "Done."
}

show_help() {
    echo "ffshim Build Script"
    echo "==================="
    echo ""
    echo "Usage: $0 [command]"
    echo ""
    echo "Commands:"
    echo "  build     Build for current platform (default)"
    echo "  install   Build and install to system library path"
    echo "  prebuilt  Build and copy to prebuilt/<os>-<arch>/"
    echo "  clean     Remove build artifacts"
    echo "  help      Show this help"
    echo ""
    echo "Environment variables:"
    echo "  CC         C compiler (default: auto-detect)"
    echo "  PREFIX     Installation prefix (default: /usr/local)"
    echo "  FFMPEG_DIR FFmpeg installation directory"
    echo ""
    echo "Detected platform: $OS_NORMALIZED-$ARCH_NORMALIZED"
    echo "Output file: $OUTPUT"
    echo ""
    echo "The shim is OPTIONAL - core ffgo functionality works without it."
    echo "Only logging callbacks and some advanced features require the shim."
}

case "${1:-build}" in
    build)
        build
        ;;
    install)
        install_lib
        ;;
    prebuilt)
        prebuilt
        ;;
    clean)
        clean
        ;;
    help|--help|-h)
        show_help
        ;;
    *)
        echo "Unknown command: $1"
        echo "Run '$0 help' for usage information."
        exit 1
        ;;
esac
