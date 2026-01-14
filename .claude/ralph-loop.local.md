---
active: false
iteration: 21
max_iterations: 0
completion_promise: null
started_at: "2026-01-14T14:02:11Z"
---

Implement the ffgo library. MANDATORY: Read /home/okecho/nonibytes/ffgo/docs/internal-design.md FIRST on EVERY iteration before doing anything else. Also read user-guide.md, ffmpeg-api-purego-research.md, purego.md in docs/. Follow TDD strictly. Exit when: all tests pass, CGO_ENABLED=0 build works, decoder/encoder/scaler work, examples work. EVERYTHING is the internal-design.md and user-guide.md docs MUST be fully implemented before you can exit. No stone must be left unturned. Implementation order: internal packages first, then shim, then low-level avutil/avcodec/avformat/swscale, then high-level API, then everything else in the internal-design.md and user-guide.md docs. FFmpeg can be in /tmp/ for testing.
