---
title: HLS Segmentation and Manifest Generation
keywords: [hls, segmentation, manifest, playlist, m3u8]
tags: [core-concept, internal, advanced]
related: [../media/stream-copy-optimization.md, ../transcoding/adaptive-bitrate.md]
source: [server/delivery/hls_segmenter.go, server/delivery/manifest_builder.go]
---

# HLS Segmentation and Manifest Generation

HLS (HTTP Live Streaming) divides media into segments and serves them with manifests (playlists). The streaming server must segment encoded streams and generate accurate manifests.

## Segmentation Strategy

Segments are typically 6-10 seconds of video. The server splits at keyframes:

```go
type HLSSegmenter struct {
    encoder         *ffgo.Encoder
    segmentDir      string
    segmentDuration time.Duration  // 6-10 seconds typical

    currentSegNum   int
    currentSegFile  *os.File
    currentSegStart int64  // PTS at segment start
    firstKeyframe   int64  // PTS of first keyframe in segment

    manifest        *HLSManifest
}

func (hs *HLSSegmenter) ProcessPacket(pkt ffgo.Packet) error {
    isKeyFrame := avcodec.IsPacketKeyFrame(pkt)
    pts := avcodec.GetPacketPTS(pkt)

    // Check if we should start a new segment
    if isKeyFrame && shouldStartNewSegment(pts, hs.currentSegStart, hs.segmentDuration) {
        if err := hs.closeSegment(); err != nil {
            return err
        }
        if err := hs.startNewSegment(pts); err != nil {
            return err
        }
    }

    // Write packet to current segment
    if err := hs.currentSegFile.WritePacket(pkt); err != nil {
        return err
    }

    // Update segment info
    if isKeyFrame && hs.firstKeyframe == 0 {
        hs.firstKeyframe = pts
    }

    return nil
}

func (hs *HLSSegmenter) shouldStartNewSegment(pts, segStart int64, dur time.Duration) bool {
    // Segment duration in PTS units
    durPTS := int64(dur.Seconds() * float64(hs.timeBase.Den) / float64(hs.timeBase.Num))
    return (pts - segStart) >= durPTS
}

func (hs *HLSSegmenter) startNewSegment(startPTS int64) error {
    hs.currentSegNum++

    filename := fmt.Sprintf("segment_%03d.m4s", hs.currentSegNum)
    file, err := os.Create(filepath.Join(hs.segmentDir, filename))
    if err != nil {
        return err
    }

    hs.currentSegFile = file
    hs.currentSegStart = startPTS
    hs.firstKeyframe = 0

    // Initialize segment encoder (stream copy mode)
    encoder := ffgo.NewEncoderToWriter(file)
    encoder.WriteHeader()
    hs.encoder = encoder

    return nil
}

func (hs *HLSSegmenter) closeSegment() error {
    if hs.currentSegFile == nil {
        return nil  // No segment open
    }

    // Finalize segment
    if err := hs.encoder.Close(); err != nil {
        hs.currentSegFile.Close()
        return err
    }

    // Record segment in manifest
    duration := segmentDuration(hs.firstKeyframe, hs.currentSegStart, hs.timeBase)
    hs.manifest.AddSegment(HLSSegment{
        URI:      fmt.Sprintf("segment_%03d.m4s", hs.currentSegNum),
        Duration: duration,
        Title:    "",
    })

    return hs.currentSegFile.Close()
}
```

## HLS Manifest Format

The manifest (playlist.m3u8) is a text file describing segments and playback parameters:

```m3u8
#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:10
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-PLAYLIST-TYPE:EVENT

#EXTINF:10.0,
segment_000.m4s
#EXTINF:10.0,
segment_001.m4s
#EXTINF:10.0,
segment_002.m4s
#EXT-X-ENDLIST
```

**Key directives:**

```
#EXT-X-VERSION:3                  # HLS version (3, 4, or 6)
#EXT-X-TARGETDURATION:10          # Max segment duration in seconds
#EXT-X-MEDIA-SEQUENCE:0           # First segment number (increments on rotation)
#EXT-X-PLAYLIST-TYPE:EVENT|VOD    # EVENT = live, VOD = complete
#EXTINF:10.0,                     # Next segment duration
#EXT-X-ENDLIST                    # Playlist complete (only in VOD)
#EXT-X-DISCONTINUITY             # Bitrate change or format change
#EXT-X-I-FRAME-ONLY               # Keyframe-only variant
```

## Manifest Generation

The server builds manifests dynamically for live streams:

```go
type HLSManifest struct {
    version          int
    targetDuration   int
    mediaSequence    int
    playlistType     string  // "EVENT", "VOD"
    segments         []HLSSegment
    discontinuities  []int   // Indices where discontinuity tag appears
    mu               sync.RWMutex
}

type HLSSegment struct {
    URI      string        // segment_000.m4s
    Duration float64       // 10.0
    Title    string        // Optional
    KeyFrame bool          // Is keyframe
    Bitrate  int64         // Optional, for bitrate variants
}

func (m *HLSManifest) AddSegment(seg HLSSegment) {
    m.mu.Lock()
    defer m.mu.Unlock()

    m.segments = append(m.segments, seg)

    // Keep sliding window for live streams (3-5 segments)
    if len(m.segments) > m.slidingWindowSize {
        m.segments = m.segments[1:]
        m.mediaSequence++
    }
}

func (m *HLSManifest) Render() string {
    m.mu.RLock()
    defer m.mu.RUnlock()

    var buf strings.Builder

    buf.WriteString("#EXTM3U\n")
    buf.WriteString(fmt.Sprintf("#EXT-X-VERSION:%d\n", m.version))
    buf.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", m.targetDuration))
    buf.WriteString(fmt.Sprintf("#EXT-X-MEDIA-SEQUENCE:%d\n", m.mediaSequence))

    if m.playlistType != "" {
        buf.WriteString(fmt.Sprintf("#EXT-X-PLAYLIST-TYPE:%s\n", m.playlistType))
    }

    for i, seg := range m.segments {
        // Add discontinuity marker if needed
        if contains(m.discontinuities, i) {
            buf.WriteString("#EXT-X-DISCONTINUITY\n")
        }

        buf.WriteString(fmt.Sprintf("#EXTINF:%.1f,\n", seg.Duration))
        buf.WriteString(seg.URI + "\n")
    }

    // For VOD, mark as complete
    if m.playlistType == "VOD" {
        buf.WriteString("#EXT-X-ENDLIST\n")
    }

    return buf.String()
}
```

## Multi-Variant Manifest

For ABR, create a master playlist referencing variants:

```go
type HLSMasterManifest struct {
    variants []HLSVariantStream
}

type HLSVariantStream struct {
    URI          string  // playlist_720p.m3u8
    Bandwidth    int64   // Bitrate
    Resolution   string  // 1280x720
    FrameRate    string  // 30
    CodecVideo   string  // avc1.42401f
    CodecAudio   string  // mp4a.40.2
}

func (m *HLSMasterManifest) Render() string {
    var buf strings.Builder

    buf.WriteString("#EXTM3U\n")
    buf.WriteString("#EXT-X-VERSION:3\n")
    buf.WriteString("#EXT-X-STREAM-INF:BANDWIDTH=5000000,RESOLUTION=1920x1080\n")
    buf.WriteString("playlist_1080p.m3u8\n")

    buf.WriteString("#EXT-X-STREAM-INF:BANDWIDTH=2500000,RESOLUTION=1280x720\n")
    buf.WriteString("playlist_720p.m3u8\n")

    buf.WriteString("#EXT-X-STREAM-INF:BANDWIDTH=1200000,RESOLUTION=854x480\n")
    buf.WriteString("playlist_480p.m3u8\n")

    return buf.String()
}
```

## Closed Captions in HLS

For subtitles, reference subtitle tracks in manifest:

```m3u8
#EXT-X-MEDIA:TYPE=CLOSED-CAPTIONS,GROUP-ID="cc",LANGUAGE="en",NAME="English"

#EXT-X-STREAM-INF:BANDWIDTH=5000000,CLOSED-CAPTIONS="cc"
variant_1080p.m3u8
```

## Server Delivery

The HTTP server serves manifests and segments:

```go
type HLSDelivery struct {
    segmentDir string
    manifests  map[string]*HLSManifest
}

func (hd *HLSDelivery) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    path := r.URL.Path

    if strings.HasSuffix(path, ".m3u8") {
        // Serve manifest
        streamName := strings.TrimSuffix(filepath.Base(path), ".m3u8")
        manifest := hd.manifests[streamName]

        w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
        w.Header().Set("Cache-Control", "no-cache")
        w.WriteString(manifest.Render())

    } else if strings.HasSuffix(path, ".m4s") {
        // Serve segment
        filename := filepath.Join(hd.segmentDir, filepath.Base(path))
        http.ServeFile(w, r, filename)

    } else if strings.HasSuffix(path, ".m4i") {
        // Serve init segment (CMAF)
        filename := filepath.Join(hd.segmentDir, filepath.Base(path))
        http.ServeFile(w, r, filename)
    }
}
```

## CMAF (Common Media Application Format)

Modern HLS uses CMAF with init segments:

```go
type CMAPSegmenter struct {
    initSegment string  // init.mp4 (contains codec info)
    segments    []string  // segment_000.m4s, segment_001.m4s
}

// Init segment is written once
func (cs *CMAPSegmenter) WriteInitSegment(encoder *ffgo.Encoder) error {
    // Header contains codec parameters
    return encoder.WriteHeader()
}

// Segments reuse the init segment
// Manifest references both:
manifest := `#EXTM3U
#EXT-X-VERSION:6
#EXT-X-TARGETDURATION:10

#EXT-X-MAP:URI="init.mp4"
#EXTINF:10.0,
segment_000.m4s
#EXTINF:10.0,
segment_001.m4s
`
```

## Live vs VOD Manifest

Live streams have rotating segments (sliding window):

```go
// Live: Keep last 3 segments
manifest.slidingWindowSize = 3
// Manifest shows: segment_10, segment_11, segment_12
// After new segment:
// Manifest shows: segment_11, segment_12, segment_13

// VOD: Keep all segments
manifest.slidingWindowSize = math.MaxInt
// Add #EXT-X-ENDLIST when stream completes
```

See [stream copy optimization](../media/stream-copy-optimization.md) for segment production and [adaptive bitrate](../transcoding/adaptive-bitrate.md) for variant coordination.
