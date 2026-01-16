---
title: Storage and Persistence
keywords: [storage, persistence, dvr, recording]
tags: [index]
---

# Storage and Persistence

This section covers storage of segments, recordings, and metadata.

## Topics to Document

- **Segment Storage** - Distributed segment caching, rotation
- **DVR/Recording** - Long-duration stream recording
- **Cloud Integration** - S3, GCS, Azure Blob Storage
- **Metadata Persistence** - Database integration for manifests
- **Cleanup Policies** - Automated segment rotation and deletion

## Common Patterns

**Segment storage hierarchy:**

```
/var/media/
├── live/
│   ├── stream1/
│   │   ├── 720p/
│   │   │   ├── segment_000.m4s
│   │   │   ├── segment_001.m4s
│   │   │   └── playlist.m3u8
│   │   ├── 480p/
│   │   └── 360p/
│   └── stream2/
├── vod/
└── archive/
```

**Recording:**

```go
// Record full stream to archive
recorder := NewRecorder("stream1", "/var/media/archive/")
recorder.Start()
// ... stream running ...
recorder.Stop()  // Produces complete MP4 or MKV
```

See root `index.md` for related topics.
