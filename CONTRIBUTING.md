# Contributing to ffgo

Thank you for your interest in contributing to ffgo! This document provides guidelines and instructions for contributing.

## Code of Conduct

Be respectful, professional, and constructive. We're all here to build something great.

## Getting Started

### Prerequisites

- Go 1.21 or later
- FFmpeg 4.x-7.x development libraries
- GCC/Clang (for building the shim)

### Setup Development Environment

```bash
# Clone the repository
git clone https://github.com/obinnaokechukwu/ffgo.git
cd ffgo

# Install FFmpeg libraries
sudo apt install ffmpeg libavcodec-dev libavformat-dev libavutil-dev libswscale-dev  # Ubuntu
brew install ffmpeg  # macOS

# Build the shim
cd shim
./build.sh
cd ..

# Run tests
go test ./...
```

## How to Contribute

### Reporting Issues

- Check existing issues first to avoid duplicates
- Provide clear reproduction steps
- Include Go version, FFmpeg version, and OS
- Attach sample media files if relevant (preferably small)

### Submitting Pull Requests

1. **Fork the repository** and create a feature branch
   ```bash
   git checkout -b feature/my-feature
   ```

2. **Follow the implementation order** (if adding new features):
   - Internal packages first (`internal/`)
   - Shim updates if needed (`shim/`)
   - Low-level bindings (`avutil/`, `avcodec/`, `avformat/`, `swscale/`)
   - High-level API (`ffgo.go`, `decoder.go`, `encoder.go`, etc.)

3. **Write tests** for all new functionality
   ```bash
   # Add tests to *_test.go files
   go test -v ./...
   ```

4. **Follow Go conventions**:
   - Run `go fmt` on all code
   - Run `go vet` to catch common issues
   - Use meaningful variable and function names
   - Document exported functions and types

5. **Test thoroughly**:
   ```bash
   # Test with CGO disabled
   CGO_ENABLED=0 go build ./...
   CGO_ENABLED=0 go test ./...
   
   # Run examples
   go run examples/decode/main.go testdata/test.mp4
   go run examples/transcode/main.go testdata/test.mp4 /tmp/output.mp4
   ```

6. **Update documentation** if needed:
   - Update `docs/user-guide.md` for user-facing changes
   - Update `docs/internal-design.md` for architecture changes
   - Update `README.md` examples if relevant

7. **Commit with clear messages**:
   ```bash
   git commit -m "Add support for X"
   # OR
   git commit -m "Fix Y bug in Z"
   ```

8. **Push and create PR**:
   ```bash
   git push origin feature/my-feature
   ```
   Then open a pull request on GitHub with a clear description.

## Development Guidelines

### Code Style

- Follow standard Go style (enforced by `go fmt`)
- Keep functions focused and small
- Prefer explicit error handling over panics
- Use meaningful names over abbreviations (except standard Go idioms)

### Memory Management

- Always pair allocations with frees
- Use `defer` for cleanup
- Never store Go pointers in C memory
- Use the handle system for callbacks (see `internal/handles/`)

### Error Handling

- Return errors, don't panic (except for programmer errors)
- Use `FFmpegError` for FFmpeg-specific errors
- Provide context in error messages

### Testing

- Unit tests for pure Go code
- Integration tests for FFmpeg interactions
- Include both success and failure cases
- Test with different FFmpeg versions if possible

### Platform Considerations

Remember these platform-specific constraints:

- **Struct by value**: Only works on Darwin (macOS); use pointers elsewhere
- **Callback limit**: purego has a 2000 callback limit
- **String lifetime**: Use `runtime.KeepAlive()` for strings passed to C

See `docs/internal-design.md` section 15 for full gotcha list.

## Project Structure

```
ffgo/
├── ffgo.go              # Main public API
├── decoder.go           # Decoder implementation
├── encoder.go           # Encoder implementation
├── scaler.go            # Scaler implementation
├── frame.go             # Frame wrapper
├── io.go                # Custom I/O implementation
├── log.go               # Logging support
├── errors.go            # Error types
│
├── avutil/              # Low-level avutil bindings
├── avcodec/             # Low-level avcodec bindings
├── avformat/            # Low-level avformat bindings
├── swscale/             # Low-level swscale bindings
│
├── internal/
│   ├── bindings/        # purego function registrations
│   ├── handles/         # Go object handle system
│   └── platform/        # Platform detection
│
├── shim/                # C shim for variadic functions
├── examples/            # Working examples
├── testdata/            # Test media files
└── docs/                # Documentation
```

## Adding New Features

### Adding Low-Level Bindings

1. Add function signature to appropriate package (`avutil/`, `avcodec/`, etc.)
2. Register function in `internal/bindings/`
3. Add tests
4. Document in function comments

### Adding High-Level API

1. Design API in public package (`decoder.go`, `encoder.go`, etc.)
2. Implement using low-level bindings
3. Add comprehensive tests
4. Add example to `examples/`
5. Document in `docs/user-guide.md`

## Testing Checklist

Before submitting a PR, ensure:

- [ ] `go fmt ./...` passes
- [ ] `go vet ./...` passes
- [ ] `CGO_ENABLED=0 go build ./...` succeeds
- [ ] `CGO_ENABLED=0 go test ./...` passes
- [ ] All examples build and run
- [ ] Documentation is updated
- [ ] Commit messages are clear

## Getting Help

- **Questions**: Open a GitHub Discussion
- **Bugs**: Open a GitHub Issue
- **Documentation**: Check `docs/` directory

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
