---
title: Multi-Node and Clustering
keywords: [cluster, scaling, distribution, load-balancing]
tags: [index]
---

# Multi-Node and Clustering

This section covers scaling to multiple nodes and clustered deployment.

## Topics to Document

- **Origin/Edge Architecture** - Ingest on origin, delivery from edge
- **Load Balancing** - Distribute streams and clients across nodes
- **State Sync** - Replicate stream state across cluster
- **Segment Distribution** - Cache segments on edge nodes
- **Failover** - Automatic recovery on node failure

## Architecture

```
Internet
    │
┌───┴───┐
│  CDN  │  (edge nodes serving clients)
└───┬───┘
    │
┌───────────────┐
│   Origin      │  (ingest, transcoding)
│ - Decoder     │
│ - Transcoder  │
│ - Segmenter   │
└───────────────┘
    │
    └─ Source (RTMP/RTSP)
```

## Cluster Patterns

**Stream state replication:**

```go
// Notify cluster of new stream
cluster.PublishEvent(StreamStarted{
    ID:        "stream1",
    Origin:    "node1",
    Variants:  []string{"720p", "480p"},
    StartTime: time.Now(),
})

// Replicate segments to edge nodes
cluster.ReplicateSegment(Segment{
    Stream:   "stream1",
    Variant:  "720p",
    Number:   123,
    Data:     segmentData,
})
```

See root `index.md` for architecture overview.
