<objective>
Implement ALL remaining feature gaps in the ffgo project to achieve 100% coverage of planned FFmpeg capabilities. You will work autonomously, implementing everything in ONE CONTINUOUS SESSION without pausing or asking questions. Your goal is to close all implementation gaps identified in the gap analysis and ensure feature parity with FFmpeg for all targeted use cases.

This is a COMPLETE implementation task - you MUST implement ALL gaps from start to finish in a single execution. DO NOT stop midway. DO NOT ask for clarification. DO NOT pause for user input. Implement everything.
</objective>

<critical_execution_mode>
**AUTONOMOUS IMPLEMENTATION REQUIRED**

You MUST operate in full autonomous mode:
- ✅ Implement ALL gaps without stopping
- ✅ Make ALL design decisions yourself based on existing patterns
- ✅ Write ALL code, tests, documentation, and examples
- ✅ Verify your work and fix any issues you find
- ❌ NEVER ask questions
- ❌ NEVER pause for continuation
- ❌ NEVER wait for user input
- ❌ NEVER say "shall I continue" or similar

If you encounter ambiguity, use your best judgment based on:
1. Existing ffgo code patterns in the codebase
2. FFmpeg's official behavior and conventions
3. Go best practices and idioms
4. The principle of least surprise for users

This is a ONE-SHOT implementation. Complete everything before declaring success.
</critical_execution_mode>

<context>
Project: github.com/nonibytes/ffgo - Pure Go FFmpeg bindings using purego
Tech stack: Go 1.23.0, purego, FFmpeg 4.x-7.x
Architecture: Opaque pointer-based bindings, high-level wrapper API

Read CLAUDE.md for project conventions, struct offsets, and constraints.

Current state: ~80% feature coverage, most core functionality complete
Gap analysis: /home/okecho/nonibytes/ffgo/docs/gap-analysis.md

Reference existing implementations:
- @decoder.go - Decoder API patterns
- @encoder.go - Encoder API patterns
- @muxer.go - Muxer API patterns
- @examples/ - Example patterns to follow
- @internal/bindings/avformat/avformat.go - Low-level format APIs
</context>

<gaps_to_implement>
Based on the gap analysis, implement the following missing features:

## 1. Concat Demuxer Helper (HIGH PRIORITY)
**Status**: ❌ Not implemented
**FFmpeg capability**: Concatenate multiple input files seamlessly

Implement a high-level concat helper that:
- Takes a list of input files/paths
- Uses FFmpeg's concat demuxer protocol
- Handles automatic stream matching and continuity
- Supports both file-based and in-memory concat lists
- Provides simple API: `NewConcatDecoder(files []string, opts ...DecoderOption) (*Decoder, error)`
- Works with both video and audio streams
- Handles timestamp continuity correctly

Implementation details:
- Create concat.go in the root package
- Generate concat protocol input (either temp file or custom I/O)
- Support options for unsafe concatenation (different formats/codecs)
- Include comprehensive tests in concat_test.go
- Add example in examples/concat/main.go
- Document in README.md

## 2. Advanced Format Probing Enhancements
**Status**: ⚠️ Partially implemented
**Current**: Basic probe options exist
**Missing**: Probe score analysis, multiple format attempts

Enhance DecoderOptions with:
- `ProbeScore` field to get/set probe confidence
- `FormatWhitelist`/`FormatBlacklist` for format selection
- `TryMultipleFormats` option for ambiguous inputs
- Helper: `ProbeFormat(path string) (*FormatProbeResult, error)` returning detailed probe info

## 3. Multi-Program Stream Support
**Status**: ❌ No dedicated helpers
**FFmpeg capability**: MPEG-TS multi-program handling

Implement helpers for:
- Listing programs in a multi-program stream
- Selecting specific program to decode
- Getting program metadata
- API: `Decoder.Programs() []ProgramInfo` and `DecoderOptions.ProgramID`

## 4. Data Stream Support
**Status**: ❌ No helpers
**FFmpeg capability**: Arbitrary data tracks (timed metadata, etc.)

Add support for:
- Detecting data streams
- Reading data packets
- Writing data streams
- API similar to subtitle handling: `Decoder.DataStreams()`, `Decoder.ReadDataPacket()`

## 5. Color Space Explicit Control
**Status**: ⚠️ Basic support only
**Missing**: Explicit BT.601/BT.709/BT.2020 selection

Enhance Scaler with:
- Explicit colorspace matrix selection
- BT.601/BT.709/BT.2020/BT.2100 constants
- `Scaler.SetColorspace(src, dst ColorSpace)` method
- Primaries and transfer characteristics control

## 6. Streaming Output Helpers
**Status**: ⚠️ Basic via FFmpeg protocols
**Missing**: High-level RTMP/UDP/RTP streaming helpers

Implement:
- `NewStreamingEncoder(url string, opts ...EncoderOption)` for RTMP/UDP/RTP output
- `StreamingOptions` with reconnect, buffer size, timeout
- Examples for common streaming scenarios (RTMP to YouTube, etc.)

## 7. Frame Timing Utilities
**Status**: ⚠️ No dedicated helpers
**Missing**: Frame timing calculations and validation

Add utilities:
- `FrameTiming` helper struct for PTS/DTS calculations
- `ValidateTimestamps(frames []*Frame) error`
- `GenerateTimestamps(count int, timebase AVRational, fps float64) []int64`
- `FrameRateDetect(decoder *Decoder) (float64, error)` - detect actual frame rate

## 8. Buffer/Memory Utilities
**Status**: ⚠️ Basic frame management only
**Missing**: Frame pooling, zero-copy utilities

Implement:
- `FramePool` for reusing frame allocations
- Zero-copy frame wrapping from existing buffers
- Memory usage tracking and limits
- `Frame.WrapBuffer(data []byte, width, height int, format PixelFormat) error`
</gaps_to_implement>

<implementation_requirements>

### Code Quality (NON-NEGOTIABLE)
- Follow existing ffgo code style and patterns precisely
- Use opaque pointers (unsafe.Pointer) for all FFmpeg types
- Implement functional options pattern for configuration
- Include comprehensive error handling with FFmpegError types
- Add proper resource cleanup (defer Close() patterns)
- Use runtime.KeepAlive() for string lifetimes
- Handle AVRational conversions correctly

### Testing Requirements (MANDATORY)
- Write tests for EVERY new function and type
- Test success cases AND error cases
- Use CGO_ENABLED=0 for all tests
- Include edge cases (empty input, nil pointers, invalid formats)
- Test with multiple FFmpeg features (H.264, AAC, MP4, MKV)
- Achieve >80% code coverage for new code

### Documentation Requirements (MANDATORY)
- GoDoc comments for all exported types and functions
- Usage examples in doc comments
- Update README.md with new features
- Create examples/ subdirectories for complex features
- Update docs/gap-analysis.md to mark features as implemented

### Integration Requirements
- Ensure new APIs work seamlessly with existing Decoder/Encoder/Muxer
- Maintain backward compatibility (no breaking changes)
- Follow purego constraints (no cgo, no struct-by-value on Linux)
- Test on both amd64 and arm64 (use appropriate offsets)

### File Organization
- Core APIs go in root package (*.go)
- Tests go in *_test.go files
- Examples go in examples/<feature>/main.go
- Low-level bindings stay in internal/bindings/
- No new dependencies (use only existing: purego, stdlib)
</implementation_requirements>

<implementation_sequence>
Implement features in this order (follow this exact sequence):

1. **Concat Demuxer Helper** (concat.go, concat_test.go, examples/concat/)
   - This is the highest priority missing feature
   - Use FFmpeg's concat demuxer protocol
   - Support both file and in-memory concat lists
   - Write comprehensive tests

2. **Format Probing Enhancements** (probe.go, probe_test.go)
   - Add ProbeFormat helper
   - Enhance DecoderOptions with probe controls
   - Add format whitelist/blacklist support

3. **Multi-Program Stream Support** (program.go, program_test.go)
   - Add ProgramInfo type
   - Add Decoder.Programs() method
   - Add program selection via DecoderOptions

4. **Data Stream Support** (datastream.go, datastream_test.go)
   - Add data stream detection
   - Add data packet reading/writing
   - Follow subtitle stream pattern

5. **Color Space Explicit Control** (colorspace.go, colorspace_test.go)
   - Add ColorSpace type and constants
   - Enhance Scaler with colorspace methods
   - Add tests for BT.601/709/2020

6. **Streaming Output Helpers** (streaming.go, streaming_test.go, examples/streaming/)
   - Add NewStreamingEncoder
   - Add StreamingOptions
   - Create RTMP example

7. **Frame Timing Utilities** (timing.go, timing_test.go)
   - Add FrameTiming helper
   - Add timestamp validation
   - Add frame rate detection

8. **Buffer/Memory Utilities** (pool.go, pool_test.go)
   - Add FramePool implementation
   - Add zero-copy frame wrapping
   - Add memory tracking
</implementation_sequence>

<verification_protocol>
After implementing ALL features, you MUST verify:

1. **Compilation**:
   ```bash
   CGO_ENABLED=0 go build ./...
   ```

2. **All Tests Pass**:
   ```bash
   CGO_ENABLED=0 go test -v ./...
   ```

3. **Examples Work**:
   ```bash
   cd examples/concat && go build
   cd examples/streaming && go build
   # Test other examples as needed
   ```

4. **Documentation Updated**:
   - README.md lists new features
   - docs/gap-analysis.md shows 100% coverage
   - All functions have GoDoc comments

5. **No Breaking Changes**:
   - Existing examples still compile
   - Existing tests still pass
   - No changes to public API signatures

If ANY verification step fails, FIX IT immediately and re-verify. Do not declare completion until ALL verifications pass.
</verification_protocol>

<success_criteria>
You have successfully completed this task when ALL of the following are true:

✅ All 8 feature gaps are fully implemented
✅ Each feature has comprehensive tests (>80% coverage)
✅ All tests pass with CGO_ENABLED=0
✅ All examples compile and demonstrate the features
✅ Documentation is complete (GoDoc, README, gap analysis)
✅ Code follows existing ffgo patterns and style
✅ No compilation errors or warnings
✅ No breaking changes to existing API
✅ Gap analysis document shows 100% coverage
✅ You have run the verification protocol and all checks pass

Do not stop until ALL criteria are met. This is a complete, autonomous implementation.
</success_criteria>

<autonomous_decision_making>
When you encounter decisions, use these guidelines:

**API Design**: Mirror existing patterns (Decoder, Encoder, Muxer styles)
**Naming**: Follow Go conventions and existing ffgo naming
**Error Handling**: Use FFmpegError with context, check IsEOF/IsAgain
**Options**: Use functional options pattern (SomeOption func(*Config))
**Resources**: Always provide Close() methods, document cleanup requirements
**Testing**: Mirror existing test patterns in *_test.go files
**Examples**: Keep simple, focus on demonstrating one concept clearly
**Performance**: Prefer correctness over premature optimization
**Compatibility**: When in doubt, match FFmpeg CLI behavior

DO NOT ask about these decisions. Make them autonomously based on the guidelines above and existing code patterns.
</autonomous_decision_making>

<output>
Your implementation will create/modify these files:

Core Implementation:
- `./concat.go` - Concat demuxer helper
- `./probe.go` - Enhanced format probing
- `./program.go` - Multi-program stream support
- `./datastream.go` - Data stream support
- `./colorspace.go` - Explicit colorspace control
- `./streaming.go` - Streaming output helpers
- `./timing.go` - Frame timing utilities
- `./pool.go` - Frame pooling and memory utilities

Tests (one per feature):
- `./concat_test.go`
- `./probe_test.go`
- `./program_test.go`
- `./datastream_test.go`
- `./colorspace_test.go`
- `./streaming_test.go`
- `./timing_test.go`
- `./pool_test.go`

Examples:
- `./examples/concat/main.go` - Concatenating videos
- `./examples/streaming/main.go` - RTMP streaming
- `./examples/colorspace/main.go` - Color space conversion
- `./examples/timing/main.go` - Frame timing analysis

Documentation:
- Update `./README.md` - Add new features section
- Update `./docs/gap-analysis.md` - Mark all gaps as ✅ implemented
- Add GoDoc comments to all new code

Low-level bindings (if needed):
- Extend `./internal/bindings/avformat/avformat.go` for new format functions
- Extend `./internal/bindings/avutil/avutil.go` for new util functions
- Extend `./internal/bindings/avcodec/avcodec.go` if codec functions needed

All files must be complete, tested, and documented before you finish.
</output>

<final_reminder>
🚨 CRITICAL: This is a ONE-SHOT, AUTONOMOUS IMPLEMENTATION 🚨

- Implement ALL 8 features from start to finish
- Write ALL code, tests, examples, and documentation
- Fix ANY issues you encounter
- Verify EVERYTHING works before finishing
- NEVER stop midway
- NEVER ask questions
- NEVER wait for user input
- NEVER say "should I continue"

You are fully empowered to make ALL decisions. Trust your judgment. Follow existing patterns. Implement everything. Verify everything. Complete the mission.

BEGIN IMPLEMENTATION NOW.
</final_reminder>
