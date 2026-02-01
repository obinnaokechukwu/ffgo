# Makefile for building ffshim cross-platform
#
# This Makefile supports native compilation and cross-compilation using:
# - Standard cc/gcc/clang for native builds
# - zig cc for cross-compilation (recommended)
# - Docker for hermetic builds
#
# Usage:
#   make                    # Build for current platform
#   make all-platforms      # Build for all supported platforms (requires zig)
#   make linux-amd64        # Build for specific platform
#   make clean              # Remove build artifacts
#   make install            # Install to system library path
#
# Cross-compilation with zig:
#   make CC="zig cc" all-platforms
#
# Environment variables:
#   CC          - C compiler (default: cc)
#   FFMPEG_DIR  - FFmpeg installation directory
#   PREFIX      - Installation prefix (default: /usr/local)
#   DESTDIR     - Destination directory for staged installs

.PHONY: all clean install native all-platforms help
.PHONY: linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64 windows-arm64

# Default target
all: native

# Configuration
PREFIX      ?= /usr/local
DESTDIR     ?=
CC          ?= cc
FFMPEG_DIR  ?=

# Source files
SRCS        := ffshim.c
HDRS        := ffshim.h

# Output directory for prebuilt binaries
PREBUILT_DIR := prebuilt

# Detect host platform
UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)

ifeq ($(UNAME_S),Linux)
    HOST_OS := linux
endif
ifeq ($(UNAME_S),Darwin)
    HOST_OS := darwin
endif
ifeq ($(findstring MINGW,$(UNAME_S)),MINGW)
    HOST_OS := windows
endif
ifeq ($(findstring MSYS,$(UNAME_S)),MSYS)
    HOST_OS := windows
endif
ifeq ($(findstring CYGWIN,$(UNAME_S)),CYGWIN)
    HOST_OS := windows
endif

ifeq ($(UNAME_M),x86_64)
    HOST_ARCH := amd64
endif
ifeq ($(UNAME_M),amd64)
    HOST_ARCH := amd64
endif
ifeq ($(UNAME_M),aarch64)
    HOST_ARCH := arm64
endif
ifeq ($(UNAME_M),arm64)
    HOST_ARCH := arm64
endif

# Default flags
CFLAGS_BASE := -Wall -Wextra -O2 -fPIC

# pkg-config based flags (for native builds)
PKG_LIBS := libavutil libavcodec libavformat
HAVE_AVDEVICE := $(shell pkg-config --exists libavdevice 2>/dev/null && echo yes || echo no)
ifeq ($(HAVE_AVDEVICE),yes)
    PKG_LIBS += libavdevice
    CFLAGS_BASE += -DFFSHIM_HAVE_AVDEVICE=1
endif

PKG_CFLAGS := $(shell pkg-config --cflags $(PKG_LIBS) 2>/dev/null)
PKG_LDFLAGS := $(shell pkg-config --libs $(PKG_LIBS) 2>/dev/null)

# Append pkg-config flags for native builds
CFLAGS_NATIVE := $(CFLAGS_BASE) $(PKG_CFLAGS)
LDFLAGS_NATIVE := $(PKG_LDFLAGS)

# =========================================================================
# Native build (current platform)
# =========================================================================

native:
	@echo "Building ffshim for $(HOST_OS)-$(HOST_ARCH)..."
ifeq ($(HOST_OS),linux)
	$(CC) -shared -fPIC $(CFLAGS_NATIVE) -o libffshim.so $(SRCS) $(LDFLAGS_NATIVE)
	@echo "Built: libffshim.so"
endif
ifeq ($(HOST_OS),darwin)
	$(CC) -shared -fPIC $(CFLAGS_NATIVE) -o libffshim.dylib $(SRCS) $(LDFLAGS_NATIVE)
	@echo "Built: libffshim.dylib"
endif
ifeq ($(HOST_OS),windows)
	$(CC) -shared $(CFLAGS_NATIVE) -o ffshim.dll $(SRCS) $(LDFLAGS_NATIVE) -Wl,--export-all-symbols
	@echo "Built: ffshim.dll"
endif

# =========================================================================
# Cross-compilation targets using zig cc
# These require FFmpeg headers/libs to be available for the target platform
# =========================================================================

# Check if zig is available
ZIG_EXISTS := $(shell command -v zig 2>/dev/null)

# Common zig compiler settings
ZIG_CC := zig cc
ZIG_CFLAGS := $(CFLAGS_BASE) -I$(FFMPEG_DIR)/include

# Linux targets
linux-amd64: $(PREBUILT_DIR)/linux-amd64/libffshim.so
linux-arm64: $(PREBUILT_DIR)/linux-arm64/libffshim.so

$(PREBUILT_DIR)/linux-amd64/libffshim.so: $(SRCS) $(HDRS)
	@mkdir -p $(PREBUILT_DIR)/linux-amd64
	$(ZIG_CC) -target x86_64-linux-gnu -shared -fPIC $(ZIG_CFLAGS) \
		-o $@ $(SRCS) \
		-L$(FFMPEG_DIR)/lib/linux-amd64 -lavutil -lavcodec -lavformat
	@echo "Built: $@"

$(PREBUILT_DIR)/linux-arm64/libffshim.so: $(SRCS) $(HDRS)
	@mkdir -p $(PREBUILT_DIR)/linux-arm64
	$(ZIG_CC) -target aarch64-linux-gnu -shared -fPIC $(ZIG_CFLAGS) \
		-o $@ $(SRCS) \
		-L$(FFMPEG_DIR)/lib/linux-arm64 -lavutil -lavcodec -lavformat
	@echo "Built: $@"

# macOS targets
darwin-amd64: $(PREBUILT_DIR)/darwin-amd64/libffshim.dylib
darwin-arm64: $(PREBUILT_DIR)/darwin-arm64/libffshim.dylib

$(PREBUILT_DIR)/darwin-amd64/libffshim.dylib: $(SRCS) $(HDRS)
	@mkdir -p $(PREBUILT_DIR)/darwin-amd64
	$(ZIG_CC) -target x86_64-macos -shared -fPIC $(ZIG_CFLAGS) \
		-o $@ $(SRCS) \
		-L$(FFMPEG_DIR)/lib/darwin-amd64 -lavutil -lavcodec -lavformat
	@echo "Built: $@"

$(PREBUILT_DIR)/darwin-arm64/libffshim.dylib: $(SRCS) $(HDRS)
	@mkdir -p $(PREBUILT_DIR)/darwin-arm64
	$(ZIG_CC) -target aarch64-macos -shared -fPIC $(ZIG_CFLAGS) \
		-o $@ $(SRCS) \
		-L$(FFMPEG_DIR)/lib/darwin-arm64 -lavutil -lavcodec -lavformat
	@echo "Built: $@"

# Windows targets
windows-amd64: $(PREBUILT_DIR)/windows-amd64/ffshim.dll
windows-arm64: $(PREBUILT_DIR)/windows-arm64/ffshim.dll

$(PREBUILT_DIR)/windows-amd64/ffshim.dll: $(SRCS) $(HDRS)
	@mkdir -p $(PREBUILT_DIR)/windows-amd64
	$(ZIG_CC) -target x86_64-windows-gnu -shared $(ZIG_CFLAGS) \
		-o $@ $(SRCS) \
		-L$(FFMPEG_DIR)/lib/windows-amd64 -lavutil -lavcodec -lavformat \
		-Wl,--export-all-symbols
	@echo "Built: $@"

$(PREBUILT_DIR)/windows-arm64/ffshim.dll: $(SRCS) $(HDRS)
	@mkdir -p $(PREBUILT_DIR)/windows-arm64
	$(ZIG_CC) -target aarch64-windows-gnu -shared $(ZIG_CFLAGS) \
		-o $@ $(SRCS) \
		-L$(FFMPEG_DIR)/lib/windows-arm64 -lavutil -lavcodec -lavformat \
		-Wl,--export-all-symbols
	@echo "Built: $@"

# Build all platforms (requires zig and cross-compilation setup)
all-platforms: linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64 windows-arm64
	@echo "All platform builds complete!"
	@ls -la $(PREBUILT_DIR)/*/

# =========================================================================
# Installation
# =========================================================================

install: native
ifeq ($(HOST_OS),linux)
	install -d $(DESTDIR)$(PREFIX)/lib
	install -m 755 libffshim.so $(DESTDIR)$(PREFIX)/lib/
	ldconfig 2>/dev/null || true
endif
ifeq ($(HOST_OS),darwin)
	install -d $(DESTDIR)$(PREFIX)/lib
	install -m 755 libffshim.dylib $(DESTDIR)$(PREFIX)/lib/
endif
ifeq ($(HOST_OS),windows)
	@echo "On Windows, copy ffshim.dll to your application directory or PATH"
endif

# Install prebuilt to standard location (for CI/release)
install-prebuilt:
	install -d $(DESTDIR)$(PREFIX)/lib
ifeq ($(HOST_OS),linux)
	install -m 755 $(PREBUILT_DIR)/linux-$(HOST_ARCH)/libffshim.so $(DESTDIR)$(PREFIX)/lib/ || true
endif
ifeq ($(HOST_OS),darwin)
	install -m 755 $(PREBUILT_DIR)/darwin-$(HOST_ARCH)/libffshim.dylib $(DESTDIR)$(PREFIX)/lib/ || true
endif

# =========================================================================
# Cleaning
# =========================================================================

clean:
	rm -f libffshim.so libffshim.dylib ffshim.dll
	rm -rf $(PREBUILT_DIR)

clean-prebuilt:
	rm -rf $(PREBUILT_DIR)

# =========================================================================
# Help
# =========================================================================

help:
	@echo "ffshim Build System"
	@echo "==================="
	@echo ""
	@echo "Native builds (uses system FFmpeg):"
	@echo "  make              - Build for current platform ($(HOST_OS)-$(HOST_ARCH))"
	@echo "  make install      - Install to $(PREFIX)/lib"
	@echo ""
	@echo "Cross-compilation (requires zig cc and FFmpeg libs):"
	@echo "  make linux-amd64    - Build for Linux x86_64"
	@echo "  make linux-arm64    - Build for Linux aarch64"
	@echo "  make darwin-amd64   - Build for macOS x86_64"
	@echo "  make darwin-arm64   - Build for macOS Apple Silicon"
	@echo "  make windows-amd64  - Build for Windows x86_64"
	@echo "  make windows-arm64  - Build for Windows ARM64"
	@echo "  make all-platforms  - Build all of the above"
	@echo ""
	@echo "Environment variables:"
	@echo "  CC          - C compiler (default: cc)"
	@echo "  FFMPEG_DIR  - FFmpeg installation root for cross-compilation"
	@echo "  PREFIX      - Installation prefix (default: /usr/local)"
	@echo "  DESTDIR     - Destination for staged installs"
	@echo ""
	@echo "Cross-compilation example:"
	@echo "  FFMPEG_DIR=/path/to/ffmpeg make linux-amd64"
	@echo ""
	@echo "Cleaning:"
	@echo "  make clean          - Remove all build artifacts"
	@echo "  make clean-prebuilt - Remove only prebuilt binaries"
