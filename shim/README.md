# ffshim - FFmpeg Shim Library for ffgo

This directory contains a small C shim library that wraps FFmpeg functionality that purego cannot handle directly:

1. **Variadic functions** - `av_log()` uses `printf`-style variadic arguments
2. **Log callbacks** - FFmpeg log callbacks receive `va_list` parameters
3. **AVRational operations** - struct-by-value returns on non-Darwin platforms
4. **Chapter creation** - requires direct struct manipulation

## Important: The Shim is Optional

**Core ffgo functionality (decode, encode, transcode, scale, filter) works WITHOUT the shim.**

The shim is only required for:
- Custom log callbacks (`ffgo.SetLogCallback`)
- Log level control (`ffgo.SetLogLevel`)
- Chapter writing
- AVFrame color offset discovery
- Device enumeration (with libavdevice)

## Building the Shim

### Prerequisites

You need FFmpeg development libraries installed:

**Linux (Debian/Ubuntu):**
```bash
sudo apt install libavcodec-dev libavformat-dev libavutil-dev libavdevice-dev
```

**Linux (Fedora/RHEL):**
```bash
sudo dnf install ffmpeg-devel
```

**macOS:**
```bash
brew install ffmpeg
```

**Windows (MSYS2):**
```bash
pacman -S mingw-w64-x86_64-gcc mingw-w64-x86_64-ffmpeg pkg-config
```

### Build Commands

**Using build.sh (recommended):**
```bash
# Build for current platform
./build.sh

# Build and install to /usr/local/lib
./build.sh install

# Build and copy to prebuilt/<os>-<arch>/
./build.sh prebuilt
```

**Using Makefile:**
```bash
# Build for current platform
make

# Install
sudo make install

# Clean
make clean
```

## Using the Shim

After building, the shim library needs to be discoverable. Options:

1. **Install system-wide** (recommended for production):
   ```bash
   sudo ./build.sh install
   # or
   sudo make install
   ```

2. **Set FFGO_SHIM_DIR** (recommended for development):
   ```bash
   export FFGO_SHIM_DIR=/path/to/ffgo/shim
   ```

3. **Set library path**:
   - Linux: `export LD_LIBRARY_PATH=/path/to/ffgo/shim:$LD_LIBRARY_PATH`
   - macOS: `export DYLD_LIBRARY_PATH=/path/to/ffgo/shim:$DYLD_LIBRARY_PATH`
   - Windows: Add to `PATH`

4. **Copy to application directory**:
   - Place the shim library in the same directory as your Go executable

## Pre-built Shims

Pre-built shims are available in the `prebuilt/` directory:

```
prebuilt/
  linux-amd64/libffshim.so
  linux-arm64/libffshim.so      (if available)
  darwin-amd64/libffshim.dylib  (if available)
  darwin-arm64/libffshim.dylib  (if available)
  windows-amd64/ffshim.dll      (if available)
```

Pre-built shims may be included in releases or you can build them yourself.

## Search Paths

The shim library is searched in the following order:

1. `FFGO_SHIM_DIR` environment variable
2. `LD_LIBRARY_PATH` / `DYLD_LIBRARY_PATH` / `PATH`
3. Standard library paths (`/usr/local/lib`, `/usr/lib`, etc.)
4. Executable directory
5. Module's `shim/prebuilt/<os>-<arch>/` directory
6. Module's `shim/` directory
7. Current working directory

## Troubleshooting

### Checking Shim Status

```go
import "github.com/obinnaokechukwu/ffgo"

func main() {
    // Initialize ffgo (loads FFmpeg and shim if available)
    ffgo.Init()

    // Check shim status
    fmt.Println("Shim status:", ffgo.ShimStatus())

    // Check if logging is available
    if ffgo.IsLoggingAvailable() {
        fmt.Println("Logging is available")
    } else {
        fmt.Println("Logging not available:", ffgo.ShimBuildInstructions())
    }

    // Full diagnostics
    fmt.Println(ffgo.Diagnose())
}
```

### Common Issues

**"shim library not found"**
- Build the shim: `cd shim && ./build.sh`
- Set `FFGO_SHIM_DIR` to the directory containing the shim
- Or install it: `cd shim && ./build.sh install`

**"FFmpeg development libraries not found"**
- Install FFmpeg dev packages for your OS (see Prerequisites above)

**"failed to load shim"**
- Check that the shim matches your FFmpeg version
- Ensure FFmpeg libraries are in the library path
- On Linux: run `ldconfig` after installing

## Building for Multiple Platforms

For CI/CD and releases, use GitHub Actions to build shims on native runners. See `.github/workflows/build-shim.yml`.

For local cross-compilation (advanced), you can use `zig cc`:

```bash
# Build all platforms (requires zig and FFmpeg libs for each target)
FFMPEG_DIR=/path/to/ffmpeg-multiplatform make -C shim all-platforms
```

## Files

- `ffshim.c` - Shim implementation
- `ffshim.h` - Public API
- `build.sh` - Build script (recommended)
- `Makefile` - Alternative build system with cross-compilation support
- `prebuilt/` - Pre-built shims for various platforms
